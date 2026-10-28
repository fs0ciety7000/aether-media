//go:build linux || darwin || freebsd || openbsd || netbsd

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// cronRun implements the hidden "aether agent cron-run": started by cron,
// it rotates the log and, unless the agent already runs, starts it detached
// (new session, output appended to the log). With --daemon it is that
// detached agent: it takes the lock for its whole life, then runs.
func cronRun(args []string) error {
	fs := flag.NewFlagSet("agent cron-run", flag.ContinueOnError)
	name := fs.String("name", "aether-agent", "")
	config := fs.String("config", "", "")
	listen := fs.String("listen", "", "")
	dataDir := fs.String("data-dir", "", "")
	daemon := fs.Bool("daemon", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !serviceNameRE.MatchString(*name) {
		return fmt.Errorf("nom invalide : %q", *name)
	}
	p, err := newCronPaths(*name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.StateDir, 0o700); err != nil {
		return err
	}
	if *listen != "" {
		_ = os.Setenv("AETHER_AGENT_LISTEN", *listen)
	}
	if *dataDir != "" {
		_ = os.Setenv("AETHER_AGENT_DATA_DIR", *dataDir)
	}
	agentArgs := []string{}
	if *config != "" {
		agentArgs = append(agentArgs, "-config", *config)
	}
	if *daemon {
		lock, ok, err := tryLock(p.Lock)
		if err != nil {
			return err
		}
		if !ok {
			return nil // another instance won the race
		}
		defer func() { _ = lock.Close() }() // releases the lock
		if err := os.WriteFile(p.PID, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
			return err
		}
		defer func() { _ = os.Remove(p.PID) }()
		return runAgent(agentArgs)
	}

	rotateLog(p.Log, logMaxBytes)
	if _, running := cronAgentRunning(p); running {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logf, err := os.OpenFile(p.Log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = logf.Close() }()
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer func() { _ = devnull.Close() }()
	cmd := exec.Command(exe, append([]string{"agent", "cron-run", "--daemon", "--name", *name}, cronDaemonFlags(*config, *listen, *dataDir)...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, logf, logf
	cmd.Env = append(os.Environ(), "PATH="+cronPATH())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	fmt.Fprintf(logf, "%s cron-run : démarrage de l'agent\n", time.Now().Format(time.RFC3339))
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func cronDaemonFlags(config, listen, dataDir string) []string {
	var out []string
	for _, kv := range [][2]string{{"--config", config}, {"--listen", listen}, {"--data-dir", dataDir}} {
		if kv[1] != "" {
			out = append(out, kv[0], kv[1])
		}
	}
	return out
}

// cronPATH adds the usual per-user binary folders to cron's minimal PATH,
// so that a static ffprobe in ~/bin is found.
func cronPATH() string {
	parts := filepath.SplitList(os.Getenv("PATH"))
	if home, err := os.UserHomeDir(); err == nil {
		parts = append([]string{filepath.Join(home, "bin"), filepath.Join(home, ".local", "bin")}, parts...)
	}
	parts = append(parts, "/usr/local/bin", "/usr/bin", "/bin")
	seen := map[string]bool{}
	out := parts[:0]
	for _, p := range parts {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}

// tryLock takes an exclusive flock on path without waiting. ok is false when
// another process holds it. The lock lives as long as f stays open.
func tryLock(path string) (f *os.File, ok bool, err error) {
	f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return f, true, nil
}

// cronAgentRunning reports whether the agent holds its lock, and its PID
// (read only while the lock is held, so a stale or recycled PID is never
// trusted).
func cronAgentRunning(p cronPaths) (int, bool) {
	f, ok, err := tryLock(p.Lock)
	if err != nil {
		return 0, false
	}
	if ok {
		_ = f.Close()
		return 0, false
	}
	b, _ := os.ReadFile(p.PID)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return pid, true
}

// stopCronAgent sends SIGTERM to the lock holder and waits for the lock to
// be released (SIGKILL after timeout).
func stopCronAgent(p cronPaths, timeout time.Duration) error {
	pid, running := cronAgentRunning(p)
	if !running {
		return nil
	}
	if pid <= 1 {
		return fmt.Errorf("agent en cours d'exécution mais PID inconnu (%s)", p.PID)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, running := cronAgentRunning(p); !running {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, running := cronAgentRunning(p); running {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}

// rotateLog keeps the log under max bytes: copy to .1 (previous .1 is
// dropped), then truncate. The agent appends (O_APPEND), so truncating
// under it is safe; at most a few lines written meanwhile are lost.
func rotateLog(path string, maxBytes int64) {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() < maxBytes {
		return
	}
	src, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(path+".1", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	_, cerr := io.Copy(dst, src)
	if err := dst.Close(); err != nil || cerr != nil {
		return
	}
	_ = os.Truncate(path, 0)
}
