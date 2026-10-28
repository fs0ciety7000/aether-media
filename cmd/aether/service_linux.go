package main

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

func (s *serviceSpec) resolve(dry bool) error {
	if err := s.resolveRunAs(dry); err != nil {
		return err
	}
	if !s.User {
		if !dry {
			if _, err := user.Lookup(s.RunAs); err != nil {
				return fmt.Errorf("compte %q introuvable : créez-le ou précisez --run-as", s.RunAs)
			}
		}
		s.DataDir = "/var/lib/" + s.Name // StateDirectory
	}
	return nil
}

// unitPath is where the unit file goes.
func (s serviceSpec) unitPath() (string, error) {
	if !s.User {
		return "/etc/systemd/system/" + s.Name + ".service", nil
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "systemd", "user", s.Name+".service"), nil
}

func (s serviceSpec) systemctl(args ...string) []string {
	if s.User {
		return append([]string{"--user"}, args...)
	}
	return args
}

func installSteps(s serviceSpec) ([]step, error) {
	path, err := s.unitPath()
	if err != nil {
		return nil, err
	}
	return []step{
		writeFileStep(path, systemdUnit(s), 0o644),
		cmdStep("systemctl", s.systemctl("daemon-reload")...),
		cmdStep("systemctl", s.systemctl("enable", s.Name+".service")...),
		cmdStep("systemctl", s.systemctl("restart", s.Name+".service")...),
	}, nil
}

func uninstallSteps(s serviceSpec) ([]step, error) {
	path, err := s.unitPath()
	if err != nil {
		return nil, err
	}
	return []step{
		cmdStep("systemctl", s.systemctl("disable", "--now", s.Name+".service")...).tolerant(),
		removeFileStep(path),
		cmdStep("systemctl", s.systemctl("daemon-reload")...),
	}, nil
}

func statusSteps(s serviceSpec) ([]step, error) {
	return []step{outputStep("systemctl", s.systemctl("status", "--no-pager", s.Name+".service")...)}, nil
}

func checkPrivileges(s serviceSpec) error {
	if !s.User && os.Geteuid() != 0 {
		return errors.New("droits administrateur requis : relancez avec sudo (ou utilisez --user)")
	}
	return nil
}

func installHints(s serviceSpec) []string {
	u := ""
	if s.User {
		u = "--user "
	}
	h := []string{fmt.Sprintf("Journaux : journalctl %s-u %s -f", u, s.Name)}
	if s.User {
		h = append(h, "Pour qu'il tourne sans session ouverte : loginctl enable-linger "+os.Getenv("USER"))
	}
	return append(h, roleHints(s)...)
}
