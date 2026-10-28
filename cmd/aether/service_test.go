package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseServiceFlags(t *testing.T) {
	spec, dry, err := parseServiceFlags(roleAgent, "install", []string{"--user", "--dry-run", "--config", "agent.toml"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !dry || !spec.User || spec.Name != "aether-agent" || !filepath.IsAbs(spec.Config) {
		t.Fatalf("spec = %+v, dry = %v", spec, dry)
	}
	spec, _, err = parseServiceFlags(roleServer, "install", []string{"-name", "media"}, io.Discard)
	if err != nil || spec.Name != "media" || spec.User {
		t.Fatalf("spec = %+v, err = %v", spec, err)
	}
	if s, _, _ := parseServiceFlags(roleServer, "status", nil, io.Discard); s.Name != "aether" {
		t.Fatalf("nom par défaut = %q", s.Name)
	}
	for _, bad := range [][]string{{"--name", "../x"}, {"--name", "a b"}, {"--name", "-x"}, {"extra"}, {"--inconnu"}} {
		if _, _, err := parseServiceFlags(roleServer, "install", bad, io.Discard); err == nil {
			t.Errorf("%v : erreur attendue", bad)
		}
	}
}

func TestResolveRunAs(t *testing.T) {
	t.Setenv("SUDO_USER", "alice")
	s := serviceSpec{}
	if err := s.resolveRunAs(false); err != nil || s.RunAs != "alice" {
		t.Fatalf("RunAs = %q, err = %v", s.RunAs, err)
	}
	s = serviceSpec{RunAs: "bob"}
	if err := s.resolveRunAs(false); err != nil || s.RunAs != "bob" {
		t.Fatalf("RunAs = %q, err = %v", s.RunAs, err)
	}
	s = serviceSpec{User: true}
	if err := s.resolveRunAs(false); err != nil || s.RunAs != "" {
		t.Fatalf("--user : RunAs = %q, err = %v", s.RunAs, err)
	}
}

func TestSystemdUnitSystem(t *testing.T) {
	s := serviceSpec{Role: roleAgent, Name: "aether-agent", Exe: "/opt/aether/aether", RunAs: "media",
		DataDir: "/var/lib/aether-agent", Config: "/etc/aether/mon agent.toml"}
	u := systemdUnit(s)
	for _, want := range []string{
		"Description=Agent Aether (partage de médias avec un serveur Aether)",
		"After=network-online.target",
		`ExecStart=/opt/aether/aether agent -config "/etc/aether/mon agent.toml"`,
		"User=media",
		"StateDirectory=aether-agent",
		"Environment=AETHER_AGENT_DATA_DIR=/var/lib/aether-agent",
		"EnvironmentFile=-/etc/aether/agent.env",
		"ProtectSystem=strict",
		"ProtectHome=read-only",
		"Restart=on-failure",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(u, want+"\n") {
			t.Errorf("unité sans %q :\n%s", want, u)
		}
	}
	if strings.Contains(u, "AETHER_LOG_FILE") {
		t.Error("AETHER_LOG_FILE inutile sous systemd")
	}
}

func TestSystemdUnitUser(t *testing.T) {
	u := systemdUnit(serviceSpec{Role: roleServer, Name: "aether", User: true, Exe: "/home/a/bin/aether"})
	for _, want := range []string{"ExecStart=/home/a/bin/aether serve\n", "WantedBy=default.target\n"} {
		if !strings.Contains(u, want) {
			t.Errorf("unité sans %q :\n%s", want, u)
		}
	}
	for _, no := range []string{"User=", "StateDirectory", "ProtectSystem", "AETHER_DATA_DIR", "EnvironmentFile"} {
		if strings.Contains(u, no) {
			t.Errorf("unité utilisateur avec %q :\n%s", no, u)
		}
	}
}

func TestSystemdQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/bin/aether": "/usr/bin/aether",
		"a b":             `"a b"`,
		`x"y`:             `"x\"y"`,
		"100%":            `"100%%"`,
		"$HOME":           `"$$HOME"`,
		`c:\x`:            `"c:\\x"`,
	} {
		if got := systemdQuote(in); got != want {
			t.Errorf("systemdQuote(%q) = %s, attendu %s", in, got, want)
		}
	}
}

// The units shipped in the .deb/.rpm are the generator's output for the
// package layout: regenerate with AETHER_UPDATE_UNITS=1 go test ./cmd/aether.
func TestPackagedUnits(t *testing.T) {
	for _, s := range []serviceSpec{
		{Role: roleServer, Name: "aether", Exe: "/usr/bin/aether", RunAs: "aether", DataDir: "/var/lib/aether"},
		{Role: roleAgent, Name: "aether-agent", Exe: "/usr/bin/aether", RunAs: "aether", DataDir: "/var/lib/aether-agent"},
	} {
		path := filepath.Join("..", "..", "packaging", "systemd", s.Name+".service")
		want := systemdUnit(s)
		if os.Getenv("AETHER_UPDATE_UNITS") == "1" {
			if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s diffère du générateur (AETHER_UPDATE_UNITS=1 go test ./cmd/aether) :\n%s", path, want)
		}
	}
}

func TestLaunchdPlist(t *testing.T) {
	s := serviceSpec{Role: roleServer, Name: "aether", Exe: "/usr/local/bin/aether", RunAs: "alice",
		DataDir: "/Library/Application Support/Aether", LogFile: "/Library/Logs/Aether/aether.log", Config: "/etc/a&b.toml"}
	p := launchdPlist(s)
	// Well-formed XML.
	dec := xml.NewDecoder(strings.NewReader(p))
	dec.Strict = false
	for {
		if _, err := dec.Token(); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("XML invalide : %v\n%s", err, p)
		}
	}
	for _, want := range []string{
		"<string>io.github.fs0ciety7000.aether</string>",
		"<string>/usr/local/bin/aether</string>",
		"<string>serve</string>",
		"<string>/etc/a&amp;b.toml</string>",
		"<key>AETHER_DATA_DIR</key>",
		"<string>/Library/Application Support/Aether</string>",
		"<key>UserName</key>\n\t<string>alice</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
		"<key>StandardErrorPath</key>\n\t<string>/Library/Logs/Aether/aether.log</string>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist sans %q :\n%s", want, p)
		}
	}
	// LaunchAgent: no UserName, no data directory override.
	p = launchdPlist(serviceSpec{Role: roleAgent, Name: "aether-agent", User: true, Exe: "/opt/aether", RunAs: "alice"})
	if strings.Contains(p, "UserName") || strings.Contains(p, "EnvironmentVariables") || !strings.Contains(p, "<string>agent</string>") {
		t.Errorf("LaunchAgent inattendu :\n%s", p)
	}
}

func TestWindowsEnv(t *testing.T) {
	s := serviceSpec{Role: roleAgent, DataDir: `C:\ProgramData\Aether Agent`, LogFile: `C:\ProgramData\Aether Agent\logs\aether-agent.log`, LogEnv: true}
	env := s.env()
	if len(env) != 2 || env[0] != [2]string{"AETHER_AGENT_DATA_DIR", `C:\ProgramData\Aether Agent`} || env[1][0] != "AETHER_LOG_FILE" {
		t.Fatalf("env = %v", env)
	}
}

func TestPrintSteps(t *testing.T) {
	var buf bytes.Buffer
	printSteps(&buf, []step{writeFileStep("/x/y.service", "[Unit]\nA=b\n", 0o644), cmdStep("systemctl", "enable", "a b")})
	out := buf.String()
	for _, want := range []string{"Simulation (--dry-run)", "1. écrire /x/y.service", "   [Unit]", "2. exécuter : systemctl enable \"a b\""} {
		if !strings.Contains(out, want) {
			t.Errorf("sortie sans %q :\n%s", want, out)
		}
	}
}

func TestRunStepsTolerant(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "f.txt")
	var buf bytes.Buffer
	err := runSteps(&buf, []step{
		cmdStep("false-command-that-does-not-exist-aether").tolerant(),
		writeFileStep(p, "ok", 0o600),
		removeFileStep(filepath.Join(dir, "absent")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "ok" {
		t.Fatalf("fichier = %q", b)
	}
	if !strings.Contains(buf.String(), "ignoré") {
		t.Errorf("échec toléré non signalé :\n%s", buf.String())
	}
	if err := runSteps(io.Discard, []step{cmdStep("false-command-that-does-not-exist-aether")}); err == nil {
		t.Fatal("erreur attendue")
	}
}
