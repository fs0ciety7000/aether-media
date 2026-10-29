package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

func (s *serviceSpec) resolve(dry bool) error {
	if err := s.resolveRunAs(dry); err != nil {
		return err
	}
	if s.User {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		s.LogFile = filepath.Join(home, "Library", "Logs", "Aether", s.Name+".log")
		return nil
	}
	s.DataDir = "/Library/Application Support/Aether"
	if s.Role == roleAgent {
		s.DataDir = "/Library/Application Support/Aether Agent"
	}
	if s.Name != "aether" && s.Name != "aether-agent" {
		s.DataDir += " (" + s.Name + ")"
	}
	s.LogFile = "/Library/Logs/Aether/" + s.Name + ".log"
	return nil
}

func (s serviceSpec) plistPath() string {
	if s.User {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "LaunchAgents", s.launchdLabel()+".plist")
	}
	return "/Library/LaunchDaemons/" + s.launchdLabel() + ".plist"
}

// domain is the launchd domain: system, or the user's GUI session.
func (s serviceSpec) domain() string {
	if s.User {
		return "gui/" + strconv.Itoa(os.Getuid())
	}
	return "system"
}

func installSteps(s serviceSpec) ([]step, error) {
	steps := []step{mkdirStep(filepath.Dir(s.LogFile))}
	if !s.User {
		// launchd does not create the data directory; the service account
		// cannot create it under /Library.
		steps = append(steps, mkdirStep(s.DataDir))
		if s.RunAs != "root" {
			steps = append(steps,
				cmdStep("chown", "-R", s.RunAs, s.DataDir),
				cmdStep("chown", s.RunAs, filepath.Dir(s.LogFile)))
		}
	}
	target := s.domain() + "/" + s.launchdLabel()
	return append(steps,
		writeFileStep(s.plistPath(), launchdPlist(s), 0o644),
		cmdStep("launchctl", "bootout", target).tolerant(),
		cmdStep("launchctl", "enable", target),
		bootstrapStep(s.domain(), s.plistPath(), target),
	), nil
}

// bootstrapStep loads the plist. "launchctl bootout" returns before the old
// job is gone, and a bootstrap issued meanwhile fails with "5: Input/output
// error": wait for the old job to disappear, then retry a few times.
func bootstrapStep(domain, plist, target string) step {
	st := cmdStep("launchctl", "bootstrap", domain, plist)
	run := st.run
	st.run = func() error {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) && exec.Command("launchctl", "print", target).Run() == nil {
			time.Sleep(250 * time.Millisecond)
		}
		var err error
		for range 5 {
			if err = run(); err == nil {
				return nil
			}
			time.Sleep(time.Second)
		}
		return err
	}
	return st
}

func uninstallSteps(s serviceSpec) ([]step, error) {
	return []step{
		cmdStep("launchctl", "bootout", s.domain()+"/"+s.launchdLabel()).tolerant(),
		removeFileStep(s.plistPath()),
	}, nil
}

func statusSteps(s serviceSpec) ([]step, error) {
	return []step{outputStep("launchctl", "print", s.domain()+"/"+s.launchdLabel())}, nil
}

func checkPrivileges(s serviceSpec) error {
	if !s.User && os.Geteuid() != 0 {
		return errors.New("droits administrateur requis : relancez avec sudo (ou utilisez --user)")
	}
	return nil
}

func installHints(s serviceSpec) []string {
	h := []string{"Journaux : tail -f \"" + s.LogFile + "\""}
	if s.Role == roleAgent && !s.User {
		h = append(h, "Astuce : un LaunchAgent (--user) accède plus simplement aux dossiers protégés de macOS (Films, Musique, disques externes).")
	}
	return append(h, roleHints(s)...)
}
