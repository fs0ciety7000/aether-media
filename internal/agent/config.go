// Package agent implements the Aether agent: a small HTTP server running on a
// machine that owns media and exposing named shares to the main Aether
// server (see docs/AGENT_PROTOCOL.md).
package agent

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultListen is the address the agent listens on when none is configured.
const DefaultListen = "0.0.0.0:8097"

// DefaultDiscoveryPort is the UDP port of the LAN discovery responder.
const DefaultDiscoveryPort = 7360

// Share is a named directory exposed by the agent.
type Share struct {
	Name string `toml:"name" json:"name"`
	Path string `toml:"path" json:"path"`
}

// Config configures an agent Server.
type Config struct {
	Listen      string // default "0.0.0.0:8097"
	TLSCert     string
	TLSKey      string
	Shares      []Share // from agent.toml; combined with the shares managed from the UI
	DataDir     string  // agent.token and agent-state.json live here (0600)
	FFprobe     string  // default "ffprobe"
	ExposePaths bool    // include local share paths in /info
	// Name is the display name; empty keeps the stored one (default: the
	// machine's hostname).
	Name string
	// BrowseRoots confine /browse and the shares added from the UI; empty
	// means fsbrowse.DefaultRoots().
	BrowseRoots []string
	// DiscoveryPort is the UDP port of the LAN discovery responder: 0 means
	// DefaultDiscoveryPort, a negative value disables discovery.
	DiscoveryPort int
	Logger        *slog.Logger
}

type fileConfig struct {
	Listen      string   `toml:"listen"`
	TLSCert     string   `toml:"tls_cert"`
	TLSKey      string   `toml:"tls_key"`
	DataDir     string   `toml:"data_dir"`
	FFprobe     string   `toml:"ffprobe"`
	ExposePaths bool     `toml:"expose_paths"`
	Name        string   `toml:"name"`
	BrowseRoots []string `toml:"browse_roots"`
	// DiscoveryPort is a pointer so that an explicit 0 (disabled) differs
	// from an absent key (default port).
	DiscoveryPort *int    `toml:"discovery_port"`
	Shares        []Share `toml:"share"`
}

// LoadConfig reads an agent.toml file. Unknown keys are rejected. The
// environment variables AETHER_AGENT_LISTEN and AETHER_AGENT_DATA_DIR
// override the corresponding file values. An empty path skips the file and
// only applies defaults and environment overrides: this is the zero-config
// mode, where shares and paired servers are managed from the server's UI.
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
		Name:        strings.TrimSpace(fc.Name),
		BrowseRoots: fc.BrowseRoots,
	}
	if fc.DiscoveryPort != nil {
		cfg.DiscoveryPort = *fc.DiscoveryPort
		if cfg.DiscoveryPort == 0 {
			cfg.DiscoveryPort = -1
		}
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
	if c.DiscoveryPort == 0 {
		c.DiscoveryPort = DefaultDiscoveryPort
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
	if c.DiscoveryPort > 65535 {
		return fmt.Errorf("agent: discovery_port invalide (%d)", c.DiscoveryPort)
	}
	if len(c.Name) > maxNameLen {
		return fmt.Errorf("agent: nom trop long (%d caractères max)", maxNameLen)
	}
	for _, r := range c.BrowseRoots {
		if !filepath.IsAbs(r) {
			return fmt.Errorf("agent: browse_roots : %q n'est pas un chemin absolu", r)
		}
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
