package main

// Installation de l'agent par la crontab de l'utilisateur, sans root ni
// systemd (seedbox, hébergement mutualisé) :
//
//	aether agent install --user --method cron [--listen ADDR] [--data-dir D]
//
// Deux lignes encadrées de marqueurs sont ajoutées à la crontab : « @reboot »
// et une surveillance toutes les 5 minutes. Toutes deux lancent
// « aether agent cron-run », qui démarre l'agent détaché s'il ne tourne pas.
// L'agent garde un verrou flock sur <état>/<nom>.lock pendant toute sa vie :
// le verrou disparaît avec le processus, même tué, ce qui rend la détection
// insensible aux PID recyclés (le fichier .pid n'est lu que verrou tenu).

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Installation methods.
const (
	methodAuto    = ""
	methodSystemd = "systemd"
	methodLaunchd = "launchd"
	methodCron    = "cron"
)

const cronSchedule = "*/5 * * * *"

// logMaxBytes is the size beyond which cron-run rotates the agent log.
const logMaxBytes = 10 << 20

// cronPaths are the files of a cron installation.
type cronPaths struct {
	StateDir string // ~/.local/state/aether
	Log      string // agent.log
	Lock     string
	PID      string
}

// stateHome is $XDG_STATE_HOME or ~/.local/state.
func stateHome() (string, error) {
	if d := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(d) {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state"), nil
}

func newCronPaths(name string) (cronPaths, error) {
	base, err := stateHome()
	if err != nil {
		return cronPaths{}, err
	}
	dir := filepath.Join(base, "aether")
	logName := name + ".log"
	if name == "aether-agent" {
		logName = "agent.log"
	}
	return cronPaths{StateDir: dir, Log: filepath.Join(dir, logName), Lock: filepath.Join(dir, name+".lock"), PID: filepath.Join(dir, name+".pid")}, nil
}

func cronMarkers(name string) (begin, end string) {
	return "# BEGIN " + name + " — géré par « aether agent install --method cron », ne pas modifier",
		"# END " + name
}

// shellQuote quotes a word for /bin/sh; cron treats an unescaped % as a
// newline, so it is escaped too.
func shellQuote(s string) string {
	q := "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	return strings.ReplaceAll(q, "%", `\%`)
}

// cronRunArgs is the command line cron runs (binary included).
func (s serviceSpec) cronRunArgs() []string {
	args := []string{s.Exe, "agent", "cron-run", "--name", s.Name}
	if s.Config != "" {
		args = append(args, "--config", s.Config)
	}
	if s.Listen != "" {
		args = append(args, "--listen", s.Listen)
	}
	if s.DataDir != "" {
		args = append(args, "--data-dir", s.DataDir)
	}
	return args
}

// cronBlock renders the marked lines for spec.
func cronBlock(s serviceSpec) string {
	q := make([]string, 0, 8)
	for _, a := range s.cronRunArgs() {
		q = append(q, shellQuote(a))
	}
	cmd := strings.Join(q, " ") + " >/dev/null 2>&1"
	begin, end := cronMarkers(s.Name)
	return begin + "\n@reboot " + cmd + "\n" + cronSchedule + " " + cmd + "\n" + end + "\n"
}

// cronRemove drops the marked block of name from a crontab, keeping every
// other line (and their order) untouched. found reports a block. A begin
// marker without its end marker only loses the lines that are recognisably
// ours (the marker and the cron-run commands of name).
func cronRemove(tab, name string) (out string, found bool) {
	begin, end := cronMarkers(name)
	lines := strings.SplitAfter(tab, "\n")
	isBegin := func(t string) bool { return t == begin || strings.HasPrefix(t, "# BEGIN "+name+" ") }
	ours := "agent' 'cron-run' '--name' " + shellQuote(name)
	drop := make([]bool, len(lines))
	for i := 0; i < len(lines); i++ {
		if !isBegin(strings.TrimRight(lines[i], "\r\n")) {
			continue
		}
		found = true
		j := i + 1
		for j < len(lines) && strings.TrimRight(lines[j], "\r\n") != end {
			j++
		}
		if j < len(lines) { // well-formed block
			for k := i; k <= j; k++ {
				drop[k] = true
			}
			i = j
			continue
		}
		drop[i] = true
		for k := i + 1; k < len(lines); k++ {
			if strings.Contains(lines[k], ours) {
				drop[k] = true
			}
		}
	}
	var b strings.Builder
	for i, l := range lines {
		if !drop[i] {
			b.WriteString(l)
		}
	}
	out = b.String()
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, found
}

// cronInstall returns tab with the block of spec (replacing a previous one).
func cronInstall(tab string, s serviceSpec) string {
	out, _ := cronRemove(tab, s.Name) // ends with "\n" unless empty
	return out + cronBlock(s)
}

// crontabCmd is the crontab binary (tests put a fake one in PATH).
var crontabCmd = "crontab"

// readCrontab returns the user's crontab; a missing one is empty.
func readCrontab() (string, error) {
	if _, err := exec.LookPath(crontabCmd); err != nil {
		return "", errors.New("commande crontab introuvable : installez cron, ou utilisez systemd --user")
	}
	c := exec.Command(crontabCmd, "-l")
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		msg := strings.ToLower(errb.String())
		if strings.Contains(msg, "no crontab") || strings.Contains(msg, "pas de crontab") {
			return "", nil
		}
		return "", fmt.Errorf("crontab -l : %w : %s", err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// writeCrontab replaces the user's crontab with tab (via a file: every
// crontab implementation accepts one).
func writeCrontab(tab string) error {
	f, err := os.CreateTemp("", "aether-crontab-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(tab); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	out, err := exec.Command(crontabCmd, f.Name()).CombinedOutput()
	if err != nil {
		return fmt.Errorf("crontab : %w : %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// editCrontabStep rewrites the crontab with edit; the dry run shows the
// block only (the rest of the crontab is never changed).
func editCrontabStep(desc, content string, edit func(string) string) step {
	return step{desc: desc, content: content, run: func() error {
		tab, err := readCrontab()
		if err != nil {
			return err
		}
		next := edit(tab)
		if next == tab {
			return nil
		}
		return writeCrontab(next)
	}}
}

func cronInstallSteps(s serviceSpec, p cronPaths) []step {
	start := newCmdStep(false, s.Exe, s.cronRunArgs()[1:]...)
	start.desc = "démarrer l'agent maintenant : " + strings.TrimPrefix(start.desc, "exécuter : ")
	return []step{
		mkdirStep(p.StateDir),
		editCrontabStep("ajouter à la crontab de l'utilisateur (autres lignes inchangées)", cronBlock(s),
			func(tab string) string { return cronInstall(tab, s) }),
		start,
	}
}

func cronUninstallSteps(s serviceSpec, p cronPaths) []step {
	return []step{
		editCrontabStep("retirer de la crontab les lignes marquées « "+s.Name+" »", "",
			func(tab string) string { out, _ := cronRemove(tab, s.Name); return out }),
		{desc: "arrêter l'agent (verrou " + p.Lock + ")", run: func() error { return stopCronAgent(p, 15*time.Second) }},
	}
}

func cronStatusSteps(s serviceSpec, p cronPaths) []step {
	return []step{{desc: "état de l'agent (crontab, verrou " + p.Lock + ")", output: true, run: func() error {
		tab, err := readCrontab()
		_, installed := cronRemove(tab, s.Name)
		switch {
		case err != nil:
			fmt.Printf("crontab : %v\n", err)
		case installed:
			fmt.Println("Crontab : installé (@reboot + surveillance toutes les 5 min)")
		default:
			fmt.Println("Crontab : non installé")
		}
		if pid, running := cronAgentRunning(p); running {
			fmt.Printf("Agent : en cours d'exécution (PID %d)\n", pid)
		} else {
			fmt.Println("Agent : arrêté")
		}
		fmt.Println("Journal : " + p.Log)
		return nil
	}}}
}

func cronHints(s serviceSpec, p cronPaths) []string {
	port := "8097"
	if _, pt, ok := strings.Cut(s.Listen, ":"); ok && pt != "" {
		port = pt[strings.LastIndex(pt, ":")+1:]
	}
	return []string{
		"Méthode : crontab (@reboot + surveillance toutes les 5 min, sans root ni systemd).",
		"Journal : tail -f " + shellQuote(p.Log) + " (code d'appairage et empreinte du certificat).",
		"État : aether agent status --method cron" + nameFlag(s),
		"Appairage : dans le serveur Aether, saisissez https://<adresse-de-cette-machine>:" + port + " puis le code.",
	}
}

func nameFlag(s serviceSpec) string {
	if s.Name == "aether-agent" {
		return ""
	}
	return " --name " + s.Name
}

// userSystemdCheck reports why systemd --user cannot be used (nil: usable).
var userSystemdCheck = func() error {
	if runtime.GOOS != "linux" {
		return errors.New("systemd absent")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemctl introuvable")
	}
	c := exec.Command("systemctl", "--user", "show-environment")
	out, err := c.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if i := strings.IndexByte(msg, '\n'); i > 0 {
			msg = msg[:i]
		}
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

// chooseMethod settles s.Method: cron when asked, or automatically for a
// per-user agent on Linux when systemd --user is unavailable (install), or
// when the crontab already holds the agent (uninstall, status).
func (s *serviceSpec) chooseMethod(verb string, w func(string)) error {
	switch s.Method {
	case methodCron:
		if runtime.GOOS == "windows" {
			return errors.New("--method cron : non pris en charge sous Windows")
		}
		if s.Role != roleAgent {
			return errors.New("--method cron : réservé à l'agent (aether agent install --method cron)")
		}
		if s.RunAs != "" {
			return errors.New("--method cron installe pour l'utilisateur courant : --run-as est inutile")
		}
		s.User = true
		return nil
	case methodAuto:
	default:
		return nil
	}
	if s.Role != roleAgent || runtime.GOOS == "windows" {
		return nil
	}
	if verb == "install" {
		if runtime.GOOS != "linux" || !s.User {
			return nil
		}
		if err := userSystemdCheck(); err != nil {
			s.Method = methodCron
			w(fmt.Sprintf("systemd --user indisponible (%v) : installation par la crontab de l'utilisateur.", err))
		}
		return nil
	}
	// uninstall, status: an agent installed through the crontab is found
	// there (with or without --user).
	if s.RunAs != "" {
		return nil
	}
	if tab, err := readCrontab(); err == nil {
		if _, found := cronRemove(tab, s.Name); found {
			s.Method, s.User = methodCron, true
		}
	}
	return nil
}
