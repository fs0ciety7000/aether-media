package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPrecedence(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "aether.toml")
	if err := os.WriteFile(file, []byte("[server]\nport = 9000\nhost = \"127.0.0.1\"\n[log]\nlevel = \"debug\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AETHER_DATA_DIR", dir)
	t.Setenv("AETHER_PORT", "9100")

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9100 {
		t.Errorf("env must override file: port = %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" || cfg.Log.Level != "debug" {
		t.Errorf("file values not applied: %+v", cfg)
	}
	if cfg.Paths.Transcode != filepath.Join(dir, "cache", "transcode") {
		t.Errorf("transcode dir = %s", cfg.Paths.Transcode)
	}
}

func TestLoadMissingExplicitFile(t *testing.T) {
	t.Setenv("AETHER_DATA_DIR", t.TempDir())
	if _, err := Load(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Fatal("expected error for missing explicit config")
	}
	if _, err := Load(""); err != nil {
		t.Fatalf("implicit missing config must be fine: %v", err)
	}
}

func TestValidate(t *testing.T) {
	t.Setenv("AETHER_DATA_DIR", t.TempDir())
	t.Setenv("AETHER_LOG_LEVEL", "loud")
	if _, err := Load(""); err == nil {
		t.Fatal("expected invalid log level error")
	}
}
