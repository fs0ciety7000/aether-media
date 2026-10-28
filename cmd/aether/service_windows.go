package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func (s *serviceSpec) resolve(_ bool) error {
	if s.User {
		return errors.New("--user n'est pas pris en charge sous Windows : l'agent et le serveur s'installent comme services Windows (compte LocalSystem)")
	}
	if s.RunAs != "" {
		return errors.New("--run-as n'est pas pris en charge sous Windows (compte LocalSystem ; modifiable dans services.msc)")
	}
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	dir := "Aether"
	if s.Role == roleAgent {
		dir = "Aether Agent"
	}
	if s.Name != "aether" && s.Name != "aether-agent" {
		dir += " (" + s.Name + ")"
	}
	s.DataDir = filepath.Join(base, dir)
	s.LogFile = filepath.Join(s.DataDir, "logs", s.Name+".log")
	s.LogEnv = true
	return nil
}

func displayName(s serviceSpec) string {
	if s.Role == roleAgent {
		return "Aether Agent (" + s.Name + ")"
	}
	return "Aether (" + s.Name + ")"
}

func connectSCM() (*mgr.Mgr, error) {
	m, err := mgr.Connect()
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return nil, errors.New("droits administrateur requis : ouvrez le terminal « en tant qu'administrateur »")
	}
	return m, err
}

func installSteps(s serviceSpec) ([]step, error) {
	args := s.execArgs()
	var env []string
	for _, kv := range s.env() {
		env = append(env, kv[0]+"="+kv[1])
	}
	desc := fmt.Sprintf("créer ou mettre à jour le service Windows « %s » (lancement automatique différé, relance en cas d'échec)", s.Name)
	content := fmt.Sprintf("Commande : %s\nEnvironnement : %s\nJournal : %s\n", strings.Join(args, " "), strings.Join(env, " ; "), s.LogFile)
	return []step{
		mkdirStep(filepath.Dir(s.LogFile)),
		{desc: desc, content: content, run: func() error { return createOrUpdate(s, args[0], args[1:], env) }},
		{desc: fmt.Sprintf("démarrer le service « %s »", s.Name), run: func() error { return startService(s.Name) }},
	}, nil
}

func createOrUpdate(s serviceSpec, exe string, args, env []string) error {
	m, err := connectSCM()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	cfg := mgr.Config{
		DisplayName:      displayName(s),
		Description:      s.description(),
		StartType:        mgr.StartAutomatic,
		DelayedAutoStart: true,
		ErrorControl:     mgr.ErrorNormal,
	}
	srv, err := m.OpenService(s.Name)
	if err == nil { // already installed: stop it, then update its configuration
		_ = stopService(srv)
		cur, err := srv.Config()
		if err != nil {
			_ = srv.Close()
			return err
		}
		cfg.ServiceType = cur.ServiceType
		cfg.BinaryPathName = windowsCommandLine(exe, args)
		if err := srv.UpdateConfig(cfg); err != nil {
			_ = srv.Close()
			return err
		}
	} else {
		if srv, err = m.CreateService(s.Name, exe, cfg, args...); err != nil {
			return err
		}
	}
	defer func() { _ = srv.Close() }()
	if err := srv.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: time.Minute},
	}, 24*60*60); err != nil {
		return err
	}
	// Per-service environment (data directory, log file).
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\`+s.Name, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	return k.SetStringsValue("Environment", env)
}

func windowsCommandLine(exe string, args []string) string {
	q := []string{windows.EscapeArg(exe)}
	for _, a := range args {
		q = append(q, windows.EscapeArg(a))
	}
	return strings.Join(q, " ")
}

func startService(name string) error {
	m, err := connectSCM()
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	srv, err := m.OpenService(name)
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()
	return srv.Start()
}

// stopService asks the service to stop and waits up to 30 s.
func stopService(srv *mgr.Service) error {
	st, err := srv.Control(svc.Stop)
	if err != nil {
		return err
	}
	for deadline := time.Now().Add(30 * time.Second); st.State != svc.Stopped; {
		if time.Now().After(deadline) {
			return errors.New("le service ne s'est pas arrêté dans les 30 s")
		}
		time.Sleep(300 * time.Millisecond)
		if st, err = srv.Query(); err != nil {
			return err
		}
	}
	return nil
}

func uninstallSteps(s serviceSpec) ([]step, error) {
	return []step{
		{desc: fmt.Sprintf("arrêter puis supprimer le service Windows « %s »", s.Name), run: func() error {
			m, err := connectSCM()
			if err != nil {
				return err
			}
			defer func() { _ = m.Disconnect() }()
			srv, err := m.OpenService(s.Name)
			if err != nil {
				return fmt.Errorf("service « %s » introuvable : %w", s.Name, err)
			}
			defer func() { _ = srv.Close() }()
			_ = stopService(srv)
			return srv.Delete()
		}},
	}, nil
}

func statusSteps(s serviceSpec) ([]step, error) {
	return []step{{desc: fmt.Sprintf("interroger le service « %s »", s.Name), output: true, run: func() error {
		m, err := connectSCM()
		if err != nil {
			return err
		}
		defer func() { _ = m.Disconnect() }()
		srv, err := m.OpenService(s.Name)
		if err != nil {
			fmt.Printf("« %s » n'est pas installé.\n", s.Name)
			return nil
		}
		defer func() { _ = srv.Close() }()
		st, err := srv.Query()
		if err != nil {
			return err
		}
		cfg, _ := srv.Config()
		fmt.Printf("Service : %s (%s)\nÉtat : %s\nCommande : %s\nJournal : %s\n",
			s.Name, cfg.DisplayName, stateName(st.State), cfg.BinaryPathName, s.LogFile)
		return nil
	}}}, nil
}

func stateName(st svc.State) string {
	switch st {
	case svc.Stopped:
		return "arrêté"
	case svc.StartPending:
		return "lancement en cours"
	case svc.StopPending:
		return "arrêt en cours"
	case svc.Running:
		return "en cours d'exécution"
	case svc.Paused:
		return "en pause"
	}
	return fmt.Sprintf("état %d", st)
}

func checkPrivileges(serviceSpec) error { return nil } // reported by connectSCM

func installHints(s serviceSpec) []string {
	return append([]string{"Journal : " + s.LogFile, "Gestion : services.msc, ou sc.exe query " + s.Name}, roleHints(s)...)
}
