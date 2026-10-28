package main

import (
	"strings"
	"testing"
)

func TestLinuxPlans(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/home/alice/.config")
	sys := serviceSpec{Role: roleAgent, Name: "aether-agent", Exe: "/usr/local/bin/aether", RunAs: "alice"}
	if err := sys.resolve(true); err != nil {
		t.Fatal(err)
	}
	if sys.DataDir != "/var/lib/aether-agent" {
		t.Fatalf("DataDir = %q", sys.DataDir)
	}
	steps, err := installSteps(sys)
	if err != nil {
		t.Fatal(err)
	}
	got := descs(steps)
	want := "écrire /etc/systemd/system/aether-agent.service | exécuter : systemctl daemon-reload | exécuter : systemctl enable aether-agent.service | exécuter : systemctl restart aether-agent.service"
	if got != want {
		t.Fatalf("install système :\n%s\nattendu :\n%s", got, want)
	}

	usr := serviceSpec{Role: roleServer, Name: "aether", User: true, Exe: "/home/alice/aether"}
	if err := usr.resolve(true); err != nil {
		t.Fatal(err)
	}
	steps, _ = installSteps(usr)
	if got := descs(steps); !strings.HasPrefix(got, "écrire /home/alice/.config/systemd/user/aether.service | exécuter : systemctl --user daemon-reload") {
		t.Fatalf("install --user : %s", got)
	}
	if usr.DataDir != "" || strings.Contains(steps[0].content, "User=") {
		t.Fatalf("unité utilisateur : DataDir=%q\n%s", usr.DataDir, steps[0].content)
	}
	steps, _ = uninstallSteps(usr)
	if got := descs(steps); got != "exécuter : systemctl --user disable --now aether.service | supprimer /home/alice/.config/systemd/user/aether.service | exécuter : systemctl --user daemon-reload" || !steps[0].ignoreErr {
		t.Fatalf("uninstall --user : %s", got)
	}
	steps, _ = statusSteps(sys)
	if got := descs(steps); got != "exécuter : systemctl status --no-pager aether-agent.service" || !steps[0].output {
		t.Fatalf("status : %s", got)
	}
	if err := checkPrivileges(usr); err != nil {
		t.Fatalf("--user ne demande pas root : %v", err)
	}
}

func descs(steps []step) string {
	d := make([]string, len(steps))
	for i, s := range steps {
		d[i] = s.desc
	}
	return strings.Join(d, " | ")
}
