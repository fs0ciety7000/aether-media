// Command aether is the Aether media server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/api"
	"github.com/fs0ciety7000/aether-media/internal/config"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/version"
	"github.com/fs0ciety7000/aether-media/internal/webui"
)

const usage = `Aether — serveur multimédia self-hosted

Usage :
  aether [serve] [-config fichier.toml]   démarre le serveur (défaut)
  aether migrate [-config fichier.toml]   applique les migrations et quitte
  aether healthcheck [-addr url]          vérifie qu'un serveur répond (Docker)
  aether version                          affiche la version
`

func main() {
	cmd, args := "serve", os.Args[1:]
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}

	var err error
	switch cmd {
	case "serve":
		err = runServe(args)
	case "migrate":
		err = runMigrate(args)
	case "healthcheck":
		err = runHealthcheck(args)
	case "version":
		fmt.Printf("aether %s (%s)\n", version.Version, version.Commit)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "commande inconnue : %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func setup(name string, args []string) (config.Config, *slog.Logger, error) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	path := fs.String("config", "", "chemin du fichier de configuration TOML")
	_ = fs.Parse(args) // ExitOnError: never returns an error

	cfg, err := config.Load(*path)
	if err != nil {
		return cfg, nil, err
	}
	level, _ := cfg.LogLevel()
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler = slog.NewTextHandler(os.Stderr, opts)
	if cfg.Log.Format == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	log := slog.New(h)
	slog.SetDefault(log)
	return cfg, log, cfg.EnsureDirs()
}

func runMigrate(args []string) error {
	cfg, _, err := setup("migrate", args)
	if err != nil {
		return err
	}
	st, err := store.Open(context.Background(), cfg.DatabasePath())
	if err != nil {
		return err
	}
	return st.Close()
}

func runServe(args []string) error {
	cfg, log, err := setup("serve", args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           api.New(st, log).Handler(webui.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No WriteTimeout: media streams are long-lived responses.
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("aether démarré", "addr", srv.Addr, "version", version.Version, "data", cfg.Paths.Data)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("arrêt en cours")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runHealthcheck(args []string) error {
	fs := flag.NewFlagSet("healthcheck", flag.ExitOnError)
	addr := fs.String("addr", "http://127.0.0.1:8096", "URL de base du serveur")
	_ = fs.Parse(args) // ExitOnError: never returns an error

	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get(*addr + "/healthz")
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: statut %d", res.StatusCode)
	}
	return nil
}
