// Package agent implements the Aether agent: a small HTTP server running on a
// machine that owns media and exposing named shares to the main Aether
// server (see docs/AGENT_PROTOCOL.md).
package agent

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultListen is the address the agent listens on when none is configured.
const DefaultListen = "0.0.0.0:8097"

// Share is a named directory exposed by the agent.
type Share struct {
	Name string `toml:"name"`
	Path string `toml:"path"`
}

// Config configures an agent Server.
type Config struct {
	Listen      string // default "0.0.0.0:8097"
	TLSCert     string
	TLSKey      string
	Shares      []Share
	DataDir     string // agent.token lives here (0600)
	FFprobe     string // default "ffprobe"
	ExposePaths bool   // include local share paths in /info
	Logger      *slog.Logger
}

type fileConfig struct {
	Listen      string  `toml:"listen"`
	TLSCert     string  `toml:"tls_cert"`
	TLSKey      string  `toml:"tls_key"`
	DataDir     string  `toml:"data_dir"`
	FFprobe     string  `toml:"ffprobe"`
	ExposePaths bool    `toml:"expose_paths"`
	Shares      []Share `toml:"share"`
}

// LoadConfig reads an agent.toml file. Unknown keys are rejected. The
// environment variables AETHER_AGENT_LISTEN and AETHER_AGENT_DATA_DIR
// override the corresponding file values. An empty path skips the file and
// only applies defaults and environment overrides.
func LoadConfig(path string) (Config, error) {
	var fc fileConfig
	if path != "" {
		md, err := toml.DecodeFile(path, &fc)
		if err != nil {
			return Config{}, fmt.Errorf("agent: lecture de %s : %w", path, err)
		}
		if und := md.Undecoded(); len(und) > 0 {
			keys := make([]string, len(und))
			for i, k := range und {
				keys[i] = k.String()
			}
			sort.Strings(keys)
			return Config{}, fmt.Errorf("agent: clés inconnues dans %s : %s", path, strings.Join(keys, ", "))
		}
	}
	cfg := Config{
		Listen:      fc.Listen,
		TLSCert:     fc.TLSCert,
		TLSKey:      fc.TLSKey,
		Shares:      fc.Shares,
		DataDir:     fc.DataDir,
		FFprobe:     fc.FFprobe,
		ExposePaths: fc.ExposePaths,
	}
	if v := os.Getenv("AETHER_AGENT_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("AETHER_AGENT_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = DefaultListen
	}
	if c.FFprobe == "" {
		c.FFprobe = "ffprobe"
	}
}

func validShareName(n string) bool {
	if len(n) < 1 || len(n) > 40 {
		return false
	}
	for _, r := range n {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func (c *Config) validate() error {
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errors.New("agent: tls_cert et tls_key doivent être définis ensemble")
	}
	if len(c.Shares) == 0 {
		return errors.New("agent: aucun partage configuré ([[share]])")
	}
	seen := make(map[string]bool, len(c.Shares))
	for _, sh := range c.Shares {
		if !validShareName(sh.Name) {
			return fmt.Errorf("agent: nom de partage invalide %q (attendu [a-z0-9_-]{1,40})", sh.Name)
		}
		if seen[sh.Name] {
			return fmt.Errorf("agent: partage %q défini plusieurs fois", sh.Name)
		}
		seen[sh.Name] = true
		if sh.Path == "" {
			return fmt.Errorf("agent: partage %q sans chemin", sh.Name)
		}
	}
	return nil
}
