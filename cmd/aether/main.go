// Command aether is the Aether media server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/agent"
	"github.com/fs0ciety7000/aether-media/internal/api"
	"github.com/fs0ciety7000/aether-media/internal/config"
	"github.com/fs0ciety7000/aether-media/internal/events"
	"github.com/fs0ciety7000/aether-media/internal/images"
	"github.com/fs0ciety7000/aether-media/internal/jobs"
	"github.com/fs0ciety7000/aether-media/internal/library"
	"github.com/fs0ciety7000/aether-media/internal/media/probe"
	"github.com/fs0ciety7000/aether-media/internal/metadata"
	"github.com/fs0ciety7000/aether-media/internal/metadata/httpx"
	"github.com/fs0ciety7000/aether-media/internal/metadata/musicbrainz"
	"github.com/fs0ciety7000/aether-media/internal/metadata/nfo"
	"github.com/fs0ciety7000/aether-media/internal/metadata/refresh"
	"github.com/fs0ciety7000/aether-media/internal/metadata/tmdb"
	"github.com/fs0ciety7000/aether-media/internal/secret"
	"github.com/fs0ciety7000/aether-media/internal/sources"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
	"github.com/fs0ciety7000/aether-media/internal/stream"
	"github.com/fs0ciety7000/aether-media/internal/version"
	"github.com/fs0ciety7000/aether-media/internal/webui"
)

const usage = `Aether — serveur multimédia self-hosted

Usage :
  aether [serve] [-config fichier.toml]   démarre le serveur (défaut)
  aether migrate [-config fichier.toml]   applique les migrations et quitte
  aether library add -name N -kind K -path P [-path P2]
                                          crée une bibliothèque (movies|shows|music|mixed)
  aether library list                     liste les bibliothèques
  aether scan [-library id] [-full]       scanne (toutes les bibliothèques par défaut)
                                          et analyse les fichiers avec ffprobe
  aether agent [-config agent.toml]       mode agent : partage les médias de cette machine
                                          avec un serveur Aether distant
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
	case "scan":
		err = runScan(args)
	case "library":
		err = runLibrary(args)
	case "agent":
		err = runAgent(args)
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

func setup(name string, args []string, extra ...func(*flag.FlagSet)) (config.Config, *slog.Logger, error) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	path := fs.String("config", "", "chemin du fichier de configuration TOML")
	for _, f := range extra {
		f(fs)
	}
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

// services are the long-lived components shared by serve and scan.
type services struct {
	store     *store.Store
	hub       *events.Hub
	runner    *jobs.Runner
	scanner   *library.Scanner
	refresher *refresh.Refresher
	images    *images.Cache
	features  api.Features
	sources   *sources.Manager
}

func newServices(ctx context.Context, cfg config.Config, log *slog.Logger) (*services, error) {
	st, err := store.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return nil, err
	}
	hub := events.NewHub()
	runner := jobs.NewRunner(st, log)
	box, err := secret.Open(cfg.Paths.Data)
	if err != nil {
		return nil, err
	}
	srcs := sources.New(st, box, sources.DefaultFactory{UserAgent: userAgent()}, hub, log)
	nfo.SetFileSystem(srcs.NFOFileSystem())
	scanner := library.NewScanner(st, runner, hub, log, srcs)
	prober := probe.New(cfg.Paths.FFprobe)
	if v, err := prober.Version(ctx); err != nil {
		log.Warn("ffprobe introuvable : l'analyse des médias est désactivée", "ffprobe", cfg.Paths.FFprobe, "err", err)
	} else {
		log.Info("ffprobe détecté", "version", v)
	}
	runner.Register(library.JobScan, 1, scanner.HandleScanJob)
	runner.Register(library.JobProbe, cfg.Library.ProbeWorkers, scanner.ProbeHandler(prober))

	imgs, err := images.New(images.Options{Dir: filepath.Join(cfg.Paths.Cache, "images"), UserAgent: userAgent()})
	if err != nil {
		return nil, err
	}
	refresher, features := newRefresher(cfg, st, runner, hub, log, imgs, srcs)
	runner.Register(refresh.JobRefresh, 2, refresher.HandleRefresh)
	runner.Register(refresh.JobImage, 4, refresher.HandleImage)
	scanner.AfterScan = func(ctx context.Context, libID int64) {
		if err := refresher.EnqueueLibrary(ctx, libID); err != nil {
			log.Error("planification des métadonnées", "library", libID, "err", err)
		}
	}
	return &services{store: st, hub: hub, runner: runner, scanner: scanner, refresher: refresher, images: imgs, features: features, sources: srcs}, nil
}

func userAgent() string {
	return "Aether/" + version.Version + " ( https://github.com/fs0ciety7000/aether-media )"
}

// newRefresher assembles metadata providers: local NFO/artwork first, then
// TMDB (films, séries; needs an API key) and MusicBrainz (musique).
func newRefresher(cfg config.Config, st *store.Store, runner *jobs.Runner, hub *events.Hub, log *slog.Logger, imgs *images.Cache, srcs *sources.Manager) (*refresh.Refresher, api.Features) {
	httpCache := filepath.Join(cfg.Paths.Cache, "http")
	var remote []metadata.RemoteProvider
	tm := tmdb.New(tmdb.Config{APIKey: cfg.Metadata.TMDBAPIKey, Language: cfg.Metadata.Language},
		httpx.New(httpx.Options{UserAgent: userAgent(), RatePerSec: 20, CacheDir: filepath.Join(httpCache, "tmdb")}))
	if tm.Enabled() {
		log.Info("TMDB activé", "language", cfg.Metadata.Language)
	} else {
		log.Warn("TMDB désactivé : aucune clé API (metadata.tmdb_api_key / AETHER_TMDB_API_KEY)")
	}
	remote = append(remote, tm)
	if !cfg.Metadata.DisableMusicBrainz {
		remote = append(remote, musicbrainz.New(musicbrainz.Config{},
			httpx.New(httpx.Options{UserAgent: userAgent(), RatePerSec: 1, CacheDir: filepath.Join(httpCache, "musicbrainz")})))
	}
	ingest := func(ctx context.Context, src string) (refresh.ImageInfo, error) {
		if sources.IsRemote(src) { // artwork stored next to remote media
			u, err := srcs.InputURL(ctx, src)
			if err != nil {
				return refresh.ImageInfo{}, err
			}
			src = u
		}
		s, err := imgs.Ingest(ctx, src)
		if errors.Is(err, images.ErrNotFound) || errors.Is(err, images.ErrNotImage) ||
			errors.Is(err, images.ErrTooLarge) || errors.Is(err, images.ErrBadSource) {
			return refresh.ImageInfo{}, fmt.Errorf("%w: %w", jobs.ErrPermanent, err)
		}
		if err != nil {
			return refresh.ImageInfo{}, err
		}
		return refresh.ImageInfo{Hash: s.Hash, Width: s.Width, Height: s.Height, Blurhash: s.Blurhash, Dominant: s.Dominant}, nil
	}
	features := api.Features{TMDB: tm.Enabled(), MusicBrainz: !cfg.Metadata.DisableMusicBrainz}
	return refresh.New(st, runner, hub, log, refresh.Config{Language: cfg.Metadata.Language},
		[]metadata.LocalProvider{nfo.New()}, remote, ingest), features
}

func runServe(args []string) error {
	cfg, log, err := setup("serve", args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc, err := newServices(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = svc.store.Close() }()
	defer svc.sources.Close()
	svc.sources.ProxyBase = loopbackBase(cfg)
	if err := svc.store.BackfillRatingAges(ctx); err != nil {
		log.Warn("classification par âge", "err", err)
	}

	bgCtx, cancelBg := context.WithCancel(context.Background())
	defer cancelBg()
	if err := svc.runner.Start(bgCtx); err != nil {
		return err
	}
	go svc.sources.Monitor(bgCtx, time.Minute)
	watcher := library.NewWatcher(svc.scanner, cfg.Library.WatchDebounce.Duration, cfg.Library.ScanInterval.Duration)
	go func() {
		if err := watcher.Run(bgCtx); err != nil {
			log.Error("surveillance des bibliothèques arrêtée", "err", err)
		}
	}()

	streams := stream.NewManager(stream.Config{
		FFmpeg:        cfg.Paths.FFmpeg,
		FFprobe:       cfg.Paths.FFprobe,
		TranscodeDir:  cfg.Paths.Transcode,
		MaxTranscodes: cfg.Transcode.MaxSessions,
		HWAccel:       cfg.Transcode.HWAccel,
		VAAPIDevice:   cfg.Transcode.VAAPIDevice,
		Logger:        log,
	})
	playback := &api.Playback{
		Streams: streams, Prober: probe.New(cfg.Paths.FFprobe), FFmpeg: cfg.Paths.FFmpeg,
		SubtitleDir: filepath.Join(cfg.Paths.Cache, "subtitles"),
	}

	srv := &http.Server{
		Addr: cfg.Addr(),
		Handler: api.New(api.Deps{
			Store: svc.store, Log: log, Scanner: svc.scanner, Watcher: watcher, Hub: svc.hub,
			Refresher: svc.refresher, Images: svc.images, Features: svc.features, Playback: playback,
			Sources: svc.sources,
		}).Handler(webui.Handler()),
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
	err = srv.Shutdown(shutdownCtx)
	streams.Shutdown(shutdownCtx)
	cancelBg()
	svc.runner.Wait()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runScan(args []string) error {
	var libID int64
	var full bool
	cfg, log, err := setup("scan", args, func(fs *flag.FlagSet) {
		fs.Int64Var(&libID, "library", 0, "identifiant de la bibliothèque (0 = toutes)")
		fs.BoolVar(&full, "full", false, "réanalyse aussi les fichiers inchangés")
	})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	svc, err := newServices(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = svc.store.Close() }()
	defer svc.sources.Close()
	// SFTP files reach ffprobe through the internal proxy: serve it on a
	// loopback port for the duration of the scan.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	proxy := &http.Server{Handler: api.New(api.Deps{Store: svc.store, Log: log, Sources: svc.sources}).Handler(http.NotFoundHandler()), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = proxy.Serve(ln) }()
	defer func() { _ = proxy.Close() }()
	svc.sources.ProxyBase = "http://" + ln.Addr().String()

	libs, err := svc.store.ListLibraries(ctx)
	if err != nil {
		return err
	}
	for _, l := range libs {
		if libID != 0 && l.ID != libID {
			continue
		}
		if _, err := svc.scanner.Scan(ctx, l.ID, full); err != nil {
			return err
		}
	}
	// Drain probe jobs, then stop.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := svc.runner.Start(runCtx); err != nil {
		return err
	}
	for ctx.Err() == nil {
		stats, err := svc.store.JobStats(ctx)
		if err != nil {
			return err
		}
		pending := int64(0)
		for _, st := range stats {
			if st.Kind != library.JobScan && st.Status != "failed" {
				pending += st.N
			}
		}
		if pending == 0 {
			break
		}
		log.Info("analyse et métadonnées en cours", "jobs restants", pending)
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
	cancel()
	svc.runner.Wait()
	return ctx.Err()
}

func runLibrary(args []string) error {
	if len(args) == 0 {
		return errors.New("usage : aether library add|list")
	}
	sub, args := args[0], args[1:]
	var name, kind string
	var paths stringList
	cfg, _, err := setup("library "+sub, args, func(fs *flag.FlagSet) {
		fs.StringVar(&name, "name", "", "nom")
		fs.StringVar(&kind, "kind", "", "movies | shows | music | mixed")
		fs.Var(&paths, "path", "dossier (répétable)")
	})
	if err != nil {
		return err
	}
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	switch sub {
	case "list":
		libs, err := st.ListLibraries(ctx)
		if err != nil {
			return err
		}
		for _, l := range libs {
			ps, _ := st.ListSources(ctx, l.ID)
			fmt.Printf("%d\t%s\t%s\n", l.ID, l.Kind, l.Name)
			for _, p := range ps {
				fmt.Printf("\t[%s] %s (%s)\n", p.Kind, p.Name, p.Status)
			}
		}
		return nil
	case "add":
		if name == "" || kind == "" || len(paths) == 0 {
			return errors.New("-name, -kind et au moins un -path sont requis")
		}
		var lib db.Library
		err := st.Tx(ctx, func(q *db.Queries) error {
			var err error
			if lib, err = q.CreateLibrary(ctx, db.CreateLibraryParams{Name: name, Kind: kind}); err != nil {
				return err
			}
			for _, p := range paths {
				abs, err := filepath.Abs(p)
				if err != nil {
					return err
				}
				if _, err := q.CreateSource(ctx, db.CreateSourceParams{LibraryID: lib.ID, Kind: "local", Name: abs, Root: abs, Config: "{}"}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Printf("bibliothèque %d créée\n", lib.ID)
		return nil
	}
	return fmt.Errorf("sous-commande inconnue : %q", sub)
}

type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

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

// loopbackBase is how the server reaches itself (internal proxy for FFmpeg).
func loopbackBase(cfg config.Config) string {
	host := cfg.Server.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(cfg.Server.Port))
}

func runAgent(args []string) error {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	path := fs.String("config", "", "chemin du fichier agent.toml")
	_ = fs.Parse(args) // ExitOnError: never returns an error

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	cfg, err := agent.LoadConfig(*path)
	if err != nil {
		return err
	}
	cfg.Logger = log
	srv, err := agent.New(cfg, version.Version)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return srv.Run(ctx)
}
