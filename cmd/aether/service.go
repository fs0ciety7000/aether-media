package main

// Installation d'Aether (serveur ou agent) comme service du système :
//   aether service install|uninstall|status [options]
//   aether agent   install|uninstall|status [options]
// Linux : unité systemd (système, ou --user) ; macOS : LaunchDaemon (ou
// LaunchAgent avec --user) ; Windows : service Windows (golang.org/x/sys).
// Agent sans root ni systemd (seedbox) : crontab de l'utilisateur
// (--method cron, choisie d'office si systemd --user est indisponible).
// Les fichiers générés sont des fonctions pures (testées sur tous les OS) ;
// l'exécution est décrite par une liste d'étapes que --dry-run affiche.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

const serviceUsage = `Usage :
  aether service install|uninstall|status [options]   serveur Aether
  aether agent   install|uninstall|status [options]   agent Aether

Options :
  --user          service de l'utilisateur courant (systemd --user, LaunchAgent)
                  au lieu d'un service système (Linux, macOS)
  --method M      agent : systemd, launchd ou cron (crontab de l'utilisateur,
                  sans root ni systemd ; implique --user). Par défaut, cron est
                  choisi si systemd --user est indisponible.
  --listen ADR    agent : adresse d'écoute (ex. 0.0.0.0:23456, un port libre
                  sur une seedbox)
  --data-dir D    agent, avec --user : dossier de données
  --name NOM      nom du service (défaut : aether / aether-agent)
  --config F      fichier de configuration passé au service (-config F)
  --run-as U      compte qui exécute un service système (Linux, macOS ;
                  défaut : l'utilisateur qui lance sudo, sinon « aether »)
  --dry-run       affiche ce qui serait fait, sans rien modifier
`

// Roles.
const (
	roleServer = "server"
	roleAgent  = "agent"
)

// serviceSpec describes the service to install.
type serviceSpec struct {
	Role   string // roleServer | roleAgent
	Name   string // service / unit name
	User   bool   // per-user service instead of a system one
	Exe    string // absolute path of the aether binary
	Config string // optional -config file
	RunAs  string // account of a system service (Linux, macOS)
	// DataDir is exported to the service through Env (empty: the binary's
	// default, used by per-user services).
	DataDir string
	// LogFile receives the service's output where the service manager does
	// not keep it (launchd, Windows).
	LogFile string
	// LogEnv passes LogFile as AETHER_LOG_FILE (Windows: the binary
	// redirects its own output there).
	LogEnv bool
	// Method is the installation method ("" = the platform's default,
	// methodCron = user crontab).
	Method string
	// Listen is the agent's listening address (AETHER_AGENT_LISTEN).
	Listen string
}

var serviceNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func isServiceVerb(s string) bool {
	return s == "install" || s == "uninstall" || s == "status"
}

// runAgentCmd dispatches "aether agent install|uninstall|status" and runs
// the agent otherwise.
func runAgentCmd(args []string) error {
	if len(args) > 0 && isServiceVerb(args[0]) {
		return runServiceCmd(roleAgent, args)
	}
	if len(args) > 0 && args[0] == "cron-run" {
		return cronRun(args[1:])
	}
	return runAgent(args)
}

// runServiceCmd implements "aether service …" (role server) and
// "aether agent install|uninstall|status" (role agent).
func runServiceCmd(role string, args []string) error {
	if len(args) == 0 || !isServiceVerb(args[0]) {
		fmt.Fprint(os.Stderr, serviceUsage)
		return errors.New("action attendue : install, uninstall ou status")
	}
	verb := args[0]
	spec, dry, err := parseServiceFlags(role, verb, args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if verb != "uninstall" || !dry {
		if spec.Exe, err = executablePath(); err != nil {
			return err
		}
	}
	if err := spec.chooseMethod(verb, func(m string) { fmt.Fprintln(os.Stderr, m) }); err != nil {
		return err
	}
	var steps []step
	var cp cronPaths
	if spec.Method == methodCron {
		if cp, err = newCronPaths(spec.Name); err != nil {
			return err
		}
		switch verb {
		case "install":
			steps = cronInstallSteps(spec, cp)
		case "uninstall":
			steps = cronUninstallSteps(spec, cp)
		case "status":
			steps = cronStatusSteps(spec, cp)
		}
	} else {
		if err := spec.resolve(dry); err != nil {
			return err
		}
		switch verb {
		case "install":
			steps, err = installSteps(spec)
		case "uninstall":
			steps, err = uninstallSteps(spec)
		case "status":
			steps, err = statusSteps(spec)
		}
		if err != nil {
			return err
		}
	}
	if verb == "install" && strings.HasPrefix(spec.Exe, filepath.Clean(os.TempDir())+string(filepath.Separator)) {
		fmt.Fprintf(os.Stderr, "Attention : le binaire est dans un dossier temporaire (%s) ; copiez-le d'abord à un emplacement durable.\n", spec.Exe)
	}
	if dry {
		printSteps(os.Stdout, steps)
		return nil
	}
	if verb != "status" && spec.Method != methodCron {
		if err := checkPrivileges(spec); err != nil {
			return err
		}
	}
	if err := runSteps(os.Stdout, steps); err != nil {
		return err
	}
	switch verb {
	case "install":
		fmt.Printf("\n« %s » est installé et démarré.\n", spec.Name)
		hints := installHints(spec)
		if spec.Method == methodCron {
			hints = cronHints(spec, cp)
		}
		for _, h := range hints {
			fmt.Println(h)
		}
	case "uninstall":
		fmt.Printf("\n« %s » est désinstallé (les données sont conservées).\n", spec.Name)
	}
	return nil
}

func parseServiceFlags(role, verb string, args []string, out io.Writer) (serviceSpec, bool, error) {
	spec := serviceSpec{Role: role}
	fs := flag.NewFlagSet("aether "+verb, flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() { fmt.Fprint(out, serviceUsage) }
	fs.BoolVar(&spec.User, "user", false, "service de l'utilisateur courant")
	fs.StringVar(&spec.Name, "name", "", "nom du service")
	fs.StringVar(&spec.Config, "config", "", "fichier de configuration")
	fs.StringVar(&spec.RunAs, "run-as", "", "compte qui exécute le service système")
	fs.StringVar(&spec.Method, "method", "", "méthode d'installation (systemd, launchd, cron)")
	fs.StringVar(&spec.Listen, "listen", "", "adresse d'écoute de l'agent")
	dataDir := fs.String("data-dir", "", "dossier de données de l'agent (--user)")
	dry := fs.Bool("dry-run", false, "affiche ce qui serait fait")
	if err := fs.Parse(args); err != nil {
		return spec, false, err
	}
	if fs.NArg() > 0 {
		return spec, false, fmt.Errorf("argument inattendu : %q", fs.Arg(0))
	}
	switch spec.Method {
	case "", "auto":
		spec.Method = methodAuto
	case methodSystemd, methodLaunchd, methodCron:
	default:
		return spec, false, fmt.Errorf("méthode inconnue : %q (systemd, launchd ou cron)", spec.Method)
	}
	if spec.Method == methodCron {
		spec.User = true
	}
	if (spec.Listen != "" || *dataDir != "") && role != roleAgent {
		return spec, false, errors.New("--listen et --data-dir concernent l'agent (le serveur se règle dans sa configuration)")
	}
	if spec.Listen != "" {
		if _, _, err := net.SplitHostPort(spec.Listen); err != nil {
			return spec, false, fmt.Errorf("--listen invalide (ex. 0.0.0.0:23456) : %w", err)
		}
	}
	if *dataDir != "" {
		if !spec.User {
			return spec, false, errors.New("--data-dir : réservé aux installations --user (un service système garde ses données dans son dossier d'état)")
		}
		abs, err := filepath.Abs(*dataDir)
		if err != nil {
			return spec, false, err
		}
		spec.DataDir = abs
	}
	if spec.Name == "" {
		spec.Name = "aether"
		if role == roleAgent {
			spec.Name = "aether-agent"
		}
	}
	if !serviceNameRE.MatchString(spec.Name) {
		return spec, false, fmt.Errorf("nom de service invalide : %q (lettres, chiffres, « . », « _ », « - »)", spec.Name)
	}
	if spec.Config != "" {
		abs, err := filepath.Abs(spec.Config)
		if err != nil {
			return spec, false, err
		}
		spec.Config = abs
	}
	return spec, *dry, nil
}

// executablePath is the binary the service runs. The path the command was
// typed with is kept when it leads to the same file, so that a stable
// symlink (Homebrew's bin/aether, /usr/local/bin/aether) survives updates.
func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("chemin du binaire introuvable : %w", err)
	}
	if p, err := exec.LookPath(os.Args[0]); err == nil {
		if abs, err := filepath.Abs(p); err == nil && sameFile(abs, exe) {
			return abs, nil
		}
	}
	return exe, nil
}

func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	return err == nil && os.SameFile(fa, fb)
}

// resolveRunAs fills RunAs for a system service. In a dry run an unknown
// account is shown as a placeholder instead of failing.
func (s *serviceSpec) resolveRunAs(dry bool) error {
	if s.User || s.RunAs != "" {
		return nil
	}
	u, err := defaultRunAs()
	if err != nil && dry {
		u, err = "UTILISATEUR", nil
	}
	s.RunAs = u
	return err
}

// defaultRunAs picks the account of a system service: the user behind sudo,
// else a dedicated "aether" account if it exists.
func defaultRunAs() (string, error) {
	if u := os.Getenv("SUDO_USER"); u != "" && u != "root" {
		return u, nil
	}
	if _, err := user.Lookup("aether"); err == nil {
		return "aether", nil
	}
	return "", errors.New("compte d'exécution inconnu : précisez --run-as UTILISATEUR (évitez root)")
}

// execArgs are the service's command line, binary included.
func (s serviceSpec) execArgs() []string {
	args := []string{s.Exe, "serve"}
	if s.Role == roleAgent {
		args[1] = "agent"
	}
	if s.Config != "" {
		args = append(args, "-config", s.Config)
	}
	return args
}

// env is the environment given to the service.
func (s serviceSpec) env() [][2]string {
	var env [][2]string
	if s.DataDir != "" {
		k := "AETHER_DATA_DIR"
		if s.Role == roleAgent {
			k = "AETHER_AGENT_DATA_DIR"
		}
		env = append(env, [2]string{k, s.DataDir})
	}
	if s.Listen != "" && s.Role == roleAgent {
		env = append(env, [2]string{"AETHER_AGENT_LISTEN", s.Listen})
	}
	if s.LogFile != "" && s.LogEnv {
		env = append(env, [2]string{"AETHER_LOG_FILE", s.LogFile})
	}
	return env
}

func (s serviceSpec) description() string {
	if s.Role == roleAgent {
		return "Agent Aether (partage de médias avec un serveur Aether)"
	}
	return "Serveur multimédia Aether"
}

// launchdLabel is the launchd job label (reverse DNS).
func (s serviceSpec) launchdLabel() string { return "io.github.fs0ciety7000." + s.Name }

// ---------------------------------------------------------------- systemd

// systemdUnit renders a systemd unit. A system unit runs under RunAs with
// its data in /var/lib/<name> (StateDirectory) and a read-only view of the
// rest of the system; a user unit keeps the binary's default paths.
func systemdUnit(s serviceSpec) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("[Unit]")
	w("Description=%s", s.description())
	w("Documentation=https://github.com/fs0ciety7000/aether-media/blob/main/docs/INSTALL.md")
	w("After=network-online.target")
	w("Wants=network-online.target")
	w("")
	w("[Service]")
	w("Type=simple")
	w("ExecStart=%s", systemdQuoteArgs(s.execArgs()))
	w("Restart=on-failure")
	w("RestartSec=5")
	if !s.User {
		w("User=%s", s.RunAs)
		w("StateDirectory=%s", s.Name)
		w("StateDirectoryMode=0750")
	}
	for _, kv := range s.env() {
		w("Environment=%s", systemdQuote(kv[0]+"="+kv[1]))
	}
	envFile := "/etc/aether/aether.env"
	if s.Role == roleAgent {
		envFile = "/etc/aether/agent.env"
	}
	if !s.User {
		w("EnvironmentFile=-%s", envFile)
		w("# Durcissement : lecture seule hors du dossier de données.")
		w("NoNewPrivileges=yes")
		w("ProtectSystem=strict")
		w("ProtectHome=read-only")
		w("PrivateTmp=yes")
		w("ProtectKernelTunables=yes")
		w("ProtectKernelModules=yes")
		w("ProtectControlGroups=yes")
		w("RestrictSUIDSGID=yes")
		w("LockPersonality=yes")
		w("UMask=0027")
	}
	w("TimeoutStopSec=30")
	w("")
	w("[Install]")
	if s.User {
		w("WantedBy=default.target")
	} else {
		w("WantedBy=multi-user.target")
	}
	return b.String()
}

// systemdQuote quotes one ExecStart / Environment word when needed.
func systemdQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"'\\$%;") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `$$`, `%`, `%%`)
	return `"` + r.Replace(s) + `"`
}

func systemdQuoteArgs(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = systemdQuote(a)
	}
	return strings.Join(q, " ")
}

// ---------------------------------------------------------------- launchd

// launchdPlist renders a launchd property list (LaunchDaemon, or
// LaunchAgent for a per-user service).
func launchdPlist(s serviceSpec) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w(`<?xml version="1.0" encoding="UTF-8"?>`)
	w(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`)
	w(`<plist version="1.0">`)
	w(`<dict>`)
	w(`	<key>Label</key>`)
	w(`	<string>%s</string>`, xmlEscape(s.launchdLabel()))
	w(`	<key>ProgramArguments</key>`)
	w(`	<array>`)
	for _, a := range s.execArgs() {
		w(`		<string>%s</string>`, xmlEscape(a))
	}
	w(`	</array>`)
	// launchd starts services with PATH=/usr/bin:/bin:/usr/sbin:/sbin:
	// add Homebrew so ffmpeg / ffprobe are found.
	env := append(s.env(), [2]string{"PATH", launchdPath})
	{
		w(`	<key>EnvironmentVariables</key>`)
		w(`	<dict>`)
		for _, kv := range env {
			w(`		<key>%s</key>`, xmlEscape(kv[0]))
			w(`		<string>%s</string>`, xmlEscape(kv[1]))
		}
		w(`	</dict>`)
	}
	if !s.User && s.RunAs != "" && s.RunAs != "root" {
		w(`	<key>UserName</key>`)
		w(`	<string>%s</string>`, xmlEscape(s.RunAs))
	}
	w(`	<key>RunAtLoad</key>`)
	w(`	<true/>`)
	w(`	<key>KeepAlive</key>`)
	w(`	<dict>`)
	w(`		<key>SuccessfulExit</key>`)
	w(`		<false/>`)
	w(`	</dict>`)
	w(`	<key>ThrottleInterval</key>`)
	w(`	<integer>5</integer>`)
	w(`	<key>ProcessType</key>`)
	w(`	<string>Background</string>`)
	if s.LogFile != "" {
		w(`	<key>StandardOutPath</key>`)
		w(`	<string>%s</string>`, xmlEscape(s.LogFile))
		w(`	<key>StandardErrorPath</key>`)
		w(`	<string>%s</string>`, xmlEscape(s.LogFile))
	}
	w(`</dict>`)
	w(`</plist>`)
	return b.String()
}

// launchdPath is the PATH given to launchd services.
const launchdPath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}

// ---------------------------------------------------------------- steps

// step is one action of an installation plan.
type step struct {
	desc    string // shown before running, and by --dry-run
	content string // file content shown by --dry-run
	run     func() error
	// ignoreErr: failures are reported but do not stop the plan (e.g.
	// stopping a service that is not running).
	ignoreErr bool
	// output: the command's output is shown (status).
	output bool
}

func writeFileStep(path, content string, mode os.FileMode) step {
	return step{
		desc:    "écrire " + path,
		content: content,
		run: func() error {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			return os.WriteFile(path, []byte(content), mode)
		},
	}
}

func mkdirStep(path string) step {
	return step{desc: "créer le dossier " + path, run: func() error { return os.MkdirAll(path, 0o755) }}
}

func removeFileStep(path string) step {
	return step{
		desc: "supprimer " + path,
		run: func() error {
			err := os.Remove(path)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		},
	}
}

func cmdStep(name string, args ...string) step { return newCmdStep(false, name, args...) }

// outputStep runs a command whose output is shown as is (status); its exit
// code is not an error.
//
//nolint:unused // Linux, macOS
func outputStep(name string, args ...string) step {
	s := newCmdStep(true, name, args...)
	s.ignoreErr = true
	return s
}

func newCmdStep(output bool, name string, args ...string) step {
	q := make([]string, 0, len(args)+1)
	for _, a := range append([]string{name}, args...) {
		if strings.ContainsAny(a, " \t\"'") {
			a = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		}
		q = append(q, a)
	}
	return step{
		desc:   "exécuter : " + strings.Join(q, " "),
		output: output,
		run: func() error {
			c := exec.Command(name, args...)
			if output {
				c.Stdout, c.Stderr = os.Stdout, os.Stderr
				return c.Run()
			}
			out, err := c.CombinedOutput()
			if msg := strings.TrimSpace(string(out)); err != nil && msg != "" {
				return fmt.Errorf("%w : %s", err, msg)
			}
			return err
		},
	}
}

func (s step) tolerant() step { s.ignoreErr = true; return s }

func printSteps(w io.Writer, steps []step) {
	fmt.Fprintf(w, "%s\n", "Simulation (--dry-run) : rien n'est modifié.")
	for i, s := range steps {
		fmt.Fprintf(w, "%d. %s\n", i+1, s.desc)
		if s.content != "" {
			fmt.Fprintf(w, "%s\n", "   ----")
			for _, l := range strings.Split(strings.TrimRight(s.content, "\n"), "\n") {
				fmt.Fprintf(w, "%s\n", "   "+l)
			}
			fmt.Fprintf(w, "%s\n", "   ----")
		}
	}
}

func runSteps(w io.Writer, steps []step) error {
	for _, s := range steps {
		if !s.output {
			fmt.Fprintf(w, "%s\n", "→ "+s.desc)
		}
		if err := s.run(); err != nil {
			if s.ignoreErr {
				if !s.output {
					fmt.Fprintf(w, "  (ignoré : %v)\n", err)
				}
				continue
			}
			return fmt.Errorf("%s : %w", s.desc, err)
		}
	}
	return nil
}

func roleHints(s serviceSpec) []string {
	if s.Role == roleAgent {
		return []string{
			"Code d'appairage et empreinte du certificat : dans les journaux, ou sur https://127.0.0.1:8097 depuis cette machine.",
			"Ajoutez ensuite l'agent depuis le serveur Aether (Réglages › Bibliothèques › « Appairer un agent »).",
		}
	}
	return []string{"Interface : http://localhost:8096 (ou http://<adresse-de-cette-machine>:8096)"}
}
