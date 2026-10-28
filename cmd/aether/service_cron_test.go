//go:build linux || darwin || freebsd || openbsd || netbsd

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeCrontab puts a "crontab" script in PATH that keeps the table in a
// file (absent file = no crontab, like the real one).
func fakeCrontab(t *testing.T) (tabFile string) {
	t.Helper()
	dir := t.TempDir()
	tabFile = filepath.Join(dir, "tab")
	script := `#!/bin/sh
f="$FAKE_CRONTAB"
case "$1" in
  -l) if [ -f "$f" ]; then cat "$f"; else echo "no crontab for $USER" >&2; exit 1; fi ;;
  -r) rm -f "$f" ;;
  *) if [ -n "$FAKE_CRONTAB_FAIL" ]; then echo "crontab: refused" >&2; exit 1; fi; cp "$1" "$f" ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "crontab"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CRONTAB", tabFile)
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	return tabFile
}

func cronSpec() serviceSpec {
	return serviceSpec{Role: roleAgent, Name: "aether-agent", User: true, Method: methodCron,
		Exe: "/home/al ice/bin/aether", Listen: "0.0.0.0:23456", DataDir: "/home/al ice/.aether 100%"}
}

func TestCronBlock(t *testing.T) {
	b := cronBlock(cronSpec())
	lines := strings.Split(strings.TrimSuffix(b, "\n"), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "# BEGIN aether-agent ") || lines[3] != "# END aether-agent" {
		t.Fatalf("block:\n%s", b)
	}
	cmd := `'/home/al ice/bin/aether' 'agent' 'cron-run' '--name' 'aether-agent' '--listen' '0.0.0.0:23456' '--data-dir' '/home/al ice/.aether 100\%' >/dev/null 2>&1`
	if lines[1] != "@reboot "+cmd || lines[2] != "*/5 * * * * "+cmd {
		t.Fatalf("lines:\n%s\n%s", lines[1], lines[2])
	}
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Fatal(got)
	}
}

func TestCronMerge(t *testing.T) {
	s := cronSpec()
	other := "MAILTO=me@example.org\n# mes tâches\n0 3 * * * /home/u/backup.sh\n"
	tab := cronInstall(other, s)
	if !strings.HasPrefix(tab, other) || strings.Count(tab, "# BEGIN aether-agent") != 1 {
		t.Fatalf("install:\n%s", tab)
	}
	// Re-install replaces the block, never duplicates it.
	s2 := s
	s2.Listen = "0.0.0.0:30000"
	tab2 := cronInstall(tab, s2)
	if strings.Count(tab2, "# BEGIN aether-agent") != 1 || !strings.Contains(tab2, "30000") || strings.Contains(tab2, "23456") {
		t.Fatalf("reinstall:\n%s", tab2)
	}
	out, found := cronRemove(tab2, s.Name)
	if !found || strings.TrimRight(out, "\n") != strings.TrimRight(other, "\n") {
		t.Fatalf("remove:\n%q", out)
	}
	// Another service name's block and lines without final newline survive.
	o := s
	o.Name = "aether-agent2"
	mixed := "0 1 * * * a" + "\n" + cronBlock(o) + cronBlock(s) + "0 2 * * * b"
	out, _ = cronRemove(mixed, s.Name)
	if !strings.Contains(out, "# BEGIN aether-agent2 ") || !strings.HasPrefix(out, "0 1 * * * a\n") || !strings.HasSuffix(out, "0 2 * * * b\n") ||
		strings.Contains(out, "'--name' 'aether-agent' ") {
		t.Fatalf("mixed:\n%s", out)
	}
	// A begin marker without end only removes our own lines.
	broken := strings.Replace(cronBlock(s), "# END aether-agent\n", "", 1) + "0 4 * * * keep-me\n"
	out, found = cronRemove("0 0 * * * first\n"+broken, s.Name)
	if !found || out != "0 0 * * * first\n0 4 * * * keep-me\n" {
		t.Fatalf("unterminated:\n%q", out)
	}
	if out, found := cronRemove("", s.Name); found || out != "" {
		t.Fatal("empty crontab")
	}
}

func TestCronInstallWithFakeCrontab(t *testing.T) {
	tabFile := fakeCrontab(t)
	s := cronSpec()
	s.Exe = "/bin/true" // the "start now" step
	p, err := newCronPaths(s.Name)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p.Log) != "agent.log" || filepath.Base(p.StateDir) != "aether" {
		t.Fatalf("paths %+v", p)
	}
	// No crontab at all.
	if err := runSteps(&strings.Builder{}, cronInstallSteps(s, p)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(tabFile)
	if string(b) != cronBlock(s) {
		t.Fatalf("crontab:\n%s", b)
	}
	// Existing lines are kept, a second install does not duplicate.
	other := "# perso\n*/10 * * * * /home/u/check.sh\n"
	_ = os.WriteFile(tabFile, []byte(other+string(b)), 0o600)
	if err := runSteps(&strings.Builder{}, cronInstallSteps(s, p)); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(tabFile)
	if !strings.HasPrefix(string(b), other) || strings.Count(string(b), "@reboot") != 1 {
		t.Fatalf("crontab:\n%s", b)
	}
	// Uninstall removes only our lines (and stops nothing: not running).
	if err := runSteps(&strings.Builder{}, cronUninstallSteps(s, p)); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(tabFile)
	if string(b) != other {
		t.Fatalf("after uninstall:\n%q", b)
	}
	// Errors of crontab are reported, not swallowed.
	t.Setenv("FAKE_CRONTAB_FAIL", "1")
	if err := runSteps(&strings.Builder{}, cronInstallSteps(s, p)); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("crontab failure: %v", err)
	}
}

func TestChooseMethod(t *testing.T) {
	fakeCrontab(t)
	prev := userSystemdCheck
	t.Cleanup(func() { userSystemdCheck = prev })
	userSystemdCheck = func() error { return errors.New("Failed to connect to bus") }
	var msg string
	s := serviceSpec{Role: roleAgent, Name: "aether-agent", User: true}
	if err := s.chooseMethod("install", func(m string) { msg = m }); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" && (s.Method != methodCron || !strings.Contains(msg, "crontab") || !strings.Contains(msg, "bus")) {
		t.Fatalf("auto: %q %q", s.Method, msg)
	}
	userSystemdCheck = func() error { return nil }
	s = serviceSpec{Role: roleAgent, Name: "aether-agent", User: true}
	_ = s.chooseMethod("install", func(string) {})
	if s.Method != methodAuto {
		t.Fatalf("systemd available: %q", s.Method)
	}
	s = serviceSpec{Role: roleServer, Name: "aether", Method: methodCron}
	if err := s.chooseMethod("install", func(string) {}); err == nil {
		t.Fatal("cron for the server accepted")
	}
	if _, _, err := parseServiceFlags(roleAgent, "install", []string{"--method", "ftp"}, &strings.Builder{}); err == nil {
		t.Fatal("unknown method accepted")
	}
	sp, _, err := parseServiceFlags(roleAgent, "install", []string{"--method", "cron", "--listen", "0.0.0.0:23456", "--data-dir", "d"}, &strings.Builder{})
	if err != nil || !sp.User || sp.Listen != "0.0.0.0:23456" || !filepath.IsAbs(sp.DataDir) {
		t.Fatalf("flags: %+v %v", sp, err)
	}
	if _, _, err := parseServiceFlags(roleAgent, "install", []string{"--data-dir", "d"}, &strings.Builder{}); err == nil {
		t.Fatal("--data-dir for a system service accepted")
	}
	if _, _, err := parseServiceFlags(roleAgent, "install", []string{"--listen", "nope"}, &strings.Builder{}); err == nil {
		t.Fatal("bad --listen accepted")
	}
}

// TestHelperLockHolder is a child process holding the agent lock (run by
// TestCronLockAndStop).
func TestHelperLockHolder(t *testing.T) {
	lock := os.Getenv("AETHER_TEST_LOCK")
	if lock == "" {
		t.Skip("helper")
	}
	f, ok, err := tryLock(lock)
	if err != nil || !ok {
		os.Exit(3)
	}
	defer f.Close()
	_ = os.WriteFile(os.Getenv("AETHER_TEST_PID"), []byte(strconv.Itoa(os.Getpid())), 0o600)
	time.Sleep(time.Minute)
}

func TestCronLockAndStop(t *testing.T) {
	dir := t.TempDir()
	p := cronPaths{StateDir: dir, Lock: filepath.Join(dir, "a.lock"), PID: filepath.Join(dir, "a.pid"), Log: filepath.Join(dir, "a.log")}
	if _, running := cronAgentRunning(p); running {
		t.Fatal("running without holder")
	}
	// A stale PID file (recycled PID) is ignored without the lock.
	_ = os.WriteFile(p.PID, []byte(strconv.Itoa(os.Getpid())), 0o600)
	if _, running := cronAgentRunning(p); running {
		t.Fatal("stale pid trusted")
	}
	if err := stopCronAgent(p, time.Second); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(p.PID)
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperLockHolder$")
	cmd.Env = append(os.Environ(), "AETHER_TEST_LOCK="+p.Lock, "AETHER_TEST_PID="+p.PID)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if pid, running := cronAgentRunning(p); running && pid == cmd.Process.Pid {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("holder not seen")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := stopCronAgent(p, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("holder not stopped")
	}
	if _, running := cronAgentRunning(p); running {
		t.Fatal("still running")
	}
}

func TestRotateLog(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "agent.log")
	_ = os.WriteFile(p, []byte(strings.Repeat("x", 100)), 0o600)
	rotateLog(p, 1000)
	if fi, _ := os.Stat(p); fi.Size() != 100 {
		t.Fatal("rotated too early")
	}
	rotateLog(p, 50)
	old, _ := os.ReadFile(p + ".1")
	if fi, _ := os.Stat(p); fi.Size() != 0 || len(old) != 100 {
		t.Fatalf("rotation: %d %d", fi.Size(), len(old))
	}
}
