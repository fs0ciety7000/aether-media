// Package config loads Aether's configuration from defaults, an optional TOML
// file and AETHER_* environment variables (in increasing precedence).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server   ServerConfig   `toml:"server"`
	Paths    PathsConfig    `toml:"paths"`
	Log      LogConfig      `toml:"log"`
	Library  LibraryConfig  `toml:"library"`
	Metadata MetadataConfig `toml:"metadata"`
}

type MetadataConfig struct {
	// TMDB API key (v3) or read access token (v4). Empty disables TMDB.
	TMDBAPIKey string `toml:"tmdb_api_key"`
	// Preferred metadata language (BCP-47), default "fr-FR".
	Language string `toml:"language"`
	// Disable MusicBrainz lookups (on by default; no key needed).
	DisableMusicBrainz bool `toml:"disable_musicbrainz"`
}

type LibraryConfig struct {
	// Periodic safety-net rescan of every library (default 6h).
	ScanInterval Duration `toml:"scan_interval"`
	// Quiet period after file-system events before rescanning (default 30s).
	WatchDebounce Duration `toml:"watch_debounce"`
	// Concurrent ffprobe processes (default 2).
	ProbeWorkers int `toml:"probe_workers"`
}

// Duration decodes TOML strings such as "6h" or "30s".
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	d.Duration = v
	return err
}

type ServerConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

type PathsConfig struct {
	// Data holds the database. Cache and Transcode default to sub-directories of it.
	Data      string `toml:"data"`
	Cache     string `toml:"cache"`
	Transcode string `toml:"transcode"`
	FFmpeg    string `toml:"ffmpeg"`
	FFprobe   string `toml:"ffprobe"`
}

type LogConfig struct {
	Level  string `toml:"level"`  // debug | info | warn | error
	Format string `toml:"format"` // text | json
}

// Addr is the listen address for the HTTP server.
func (c *Config) Addr() string { return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port) }

// DatabasePath is the SQLite file location.
func (c *Config) DatabasePath() string { return filepath.Join(c.Paths.Data, "aether.db") }

func Default() Config {
	return Config{
		Server: ServerConfig{Host: "0.0.0.0", Port: 8096},
		Paths:  PathsConfig{Data: defaultDataDir(), FFmpeg: "ffmpeg", FFprobe: "ffprobe"},
		Log:    LogConfig{Level: "info", Format: "text"},
		Library: LibraryConfig{
			ScanInterval:  Duration{6 * time.Hour},
			WatchDebounce: Duration{30 * time.Second},
			ProbeWorkers:  2,
		},
		Metadata: MetadataConfig{Language: "fr-FR"},
	}
}

// Load builds the configuration. An empty path means "use <data>/aether.toml
// if it exists"; an explicit path must exist.
func Load(path string) (Config, error) {
	cfg := Default()
	if v := os.Getenv("AETHER_DATA_DIR"); v != "" {
		cfg.Paths.Data = v
	}

	explicit := path != ""
	if !explicit {
		path = os.Getenv("AETHER_CONFIG")
		explicit = path != ""
	}
	if !explicit {
		path = filepath.Join(cfg.Paths.Data, "aether.toml")
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		if explicit || !errors.Is(err, fs.ErrNotExist) {
			return cfg, fmt.Errorf("config %s: %w", path, err)
		}
	}

	if err := applyEnv(&cfg); err != nil {
		return cfg, err
	}
	if cfg.Paths.Cache == "" {
		cfg.Paths.Cache = filepath.Join(cfg.Paths.Data, "cache")
	}
	if cfg.Paths.Transcode == "" {
		cfg.Paths.Transcode = filepath.Join(cfg.Paths.Cache, "transcode")
	}
	return cfg, cfg.validate()
}

func applyEnv(cfg *Config) error {
	str := map[string]*string{
		"AETHER_HOST":          &cfg.Server.Host,
		"AETHER_DATA_DIR":      &cfg.Paths.Data,
		"AETHER_CACHE_DIR":     &cfg.Paths.Cache,
		"AETHER_TRANSCODE_DIR": &cfg.Paths.Transcode,
		"AETHER_FFMPEG":        &cfg.Paths.FFmpeg,
		"AETHER_FFPROBE":       &cfg.Paths.FFprobe,
		"AETHER_LOG_LEVEL":     &cfg.Log.Level,
		"AETHER_LOG_FORMAT":    &cfg.Log.Format,
		"AETHER_TMDB_API_KEY":  &cfg.Metadata.TMDBAPIKey,
		"AETHER_LANGUAGE":      &cfg.Metadata.Language,
	}
	for k, dst := range str {
		if v := os.Getenv(k); v != "" {
			*dst = v
		}
	}
	if v := os.Getenv("AETHER_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("AETHER_PORT: %w", err)
		}
		cfg.Server.Port = p
	}
	return nil
}

func (c *Config) validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port: %d out of range", c.Server.Port)
	}
	if _, err := c.LogLevel(); err != nil {
		return err
	}
	if c.Library.ScanInterval.Duration < time.Minute {
		return fmt.Errorf("library.scan_interval: %s (minimum 1m)", c.Library.ScanInterval)
	}
	if c.Library.ProbeWorkers < 1 || c.Library.ProbeWorkers > 32 {
		return fmt.Errorf("library.probe_workers: %d (1 à 32)", c.Library.ProbeWorkers)
	}
	if f := c.Log.Format; f != "text" && f != "json" {
		return fmt.Errorf("log.format: %q (expected text or json)", f)
	}
	return nil
}

func (c *Config) LogLevel() (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(strings.ToUpper(c.Log.Level))); err != nil {
		return l, fmt.Errorf("log.level: %w", err)
	}
	return l, nil
}

// EnsureDirs creates the data, cache and transcode directories.
func (c *Config) EnsureDirs() error {
	for _, d := range []string{c.Paths.Data, c.Paths.Cache, c.Paths.Transcode} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return err
		}
	}
	return nil
}

func defaultDataDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "Aether")
		}
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Aether")
	default:
		if v := os.Getenv("XDG_DATA_HOME"); v != "" {
			return filepath.Join(v, "aether")
		}
		return filepath.Join(home, ".local", "share", "aether")
	}
	return filepath.Join(home, ".aether")
}
