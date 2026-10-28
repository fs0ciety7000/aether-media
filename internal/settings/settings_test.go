package settings

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/config"
	"github.com/fs0ciety7000/aether-media/internal/secret"
	"github.com/fs0ciety7000/aether-media/internal/store"
)

func open(t *testing.T) (*store.Store, *secret.Box, config.Config) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AETHER_DATA_DIR", dir)
	t.Setenv("AETHER_LANGUAGE", "en-US")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secret.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return st, box, cfg
}

func ptr[T any](v T) *T { return &v }

func TestPriorityMaskingAndHooks(t *testing.T) {
	ctx := context.Background()
	st, box, cfg := open(t)
	s, err := New(ctx, st, box, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var gotKey string
	var gotMax int
	var gotEvery time.Duration
	s.SetHooks(Hooks{TMDBKey: func(k string) { gotKey = k }, MaxTranscodes: func(n int) { gotMax = n }, ScanInterval: func(d time.Duration) { gotEvery = d }})

	v, err := s.View(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.MetadataLanguage.Source != SourceEnv || !v.MetadataLanguage.ReadOnly || v.MetadataLanguage.Value != "en-US" {
		t.Fatalf("language = %+v", v.MetadataLanguage)
	}
	if v.ServerName.Source != SourceDefault || v.ServerName.Value != "Aether" || v.TMDBAPIKey.Set {
		t.Fatalf("defaults = %+v %+v", v.ServerName, v.TMDBAPIKey)
	}
	var managed *ErrManaged
	if _, err := s.Apply(ctx, Patch{MetadataLanguage: ptr("fr-FR")}); !errors.As(err, &managed) || managed.Field != "metadataLanguage" {
		t.Fatalf("managed: %v", err)
	}
	var invalid *ErrInvalid
	if _, err := s.Apply(ctx, Patch{MaxTranscodes: ptr(0)}); !errors.As(err, &invalid) {
		t.Fatalf("invalid: %v", err)
	}
	if _, err := s.Apply(ctx, Patch{HWAccel: &HWAccel{Mode: "cuda"}}); !errors.As(err, &invalid) {
		t.Fatalf("invalid hw: %v", err)
	}

	const key = "0123456789abcdef0123456789abcdef"
	v, err = s.Apply(ctx, Patch{ServerName: ptr(" Salon "), TMDBAPIKey: ptr(key), MaxTranscodes: ptr(4), ScanIntervalMinutes: ptr(0),
		HWAccel: &HWAccel{Mode: "vaapi"}, MetadataCountry: ptr("be")})
	if err != nil {
		t.Fatal(err)
	}
	if v.ServerName.Value != "Salon" || v.ServerName.Source != SourceDatabase || v.MetadataCountry.Value != "BE" {
		t.Fatalf("view = %+v", v)
	}
	if !v.TMDBAPIKey.Set || v.TMDBAPIKey.Masked != "…cdef" || v.TMDBAPIKey.Source != SourceDatabase {
		t.Fatalf("tmdb = %+v", v.TMDBAPIKey)
	}
	if !v.HWAccel.RestartRequired || v.HWAccel.Applies != AppliesRestart || v.HWAccel.Value.Device == "" {
		t.Fatalf("hw = %+v", v.HWAccel)
	}
	if gotKey != key || gotMax != 4 || gotEvery != 0 || !s.TMDBEnabled() {
		t.Fatalf("hooks: %q %d %s", gotKey, gotMax, gotEvery)
	}
	// Encrypted at rest.
	row, err := st.GetSetting(ctx, keyTMDB)
	if err != nil || bytes.Contains(row.Secret, []byte(key)) || strings.Contains(row.Value, key) {
		t.Fatalf("stored in clear: %v %q", err, row.Value)
	}
	// A restart picks the stored values up.
	s2, err := New(ctx, st, box, cfg)
	if err != nil {
		t.Fatal(err)
	}
	cur := s2.Current()
	if cur.TMDBKey != key || cur.MaxTranscodes != 4 || cur.HWAccel.Mode != "vaapi" || cur.ScanInterval != 0 {
		t.Fatalf("reloaded = %+v", cur)
	}
	over := Overlay(cfg, cur)
	if over.Metadata.TMDBAPIKey != key || over.Transcode.HWAccel != "vaapi" || over.Metadata.Language != "en-US" {
		t.Fatalf("overlay = %+v", over)
	}
	// Removing the key.
	if v, err = s.Apply(ctx, Patch{TMDBAPIKey: ptr("")}); err != nil || v.TMDBAPIKey.Set || gotKey != "" {
		t.Fatalf("remove key: %v %+v", err, v.TMDBAPIKey)
	}
}

func TestWizard(t *testing.T) {
	ctx := context.Background()
	st, box, cfg := open(t)
	s, err := New(ctx, st, box, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.WizardPending(ctx) {
		t.Fatal("pending before setup")
	}
	if err := s.StartWizard(ctx); err != nil {
		t.Fatal(err)
	}
	if w, _ := s.Wizard(ctx); !s.WizardPending(ctx) || w.Step != "metadata" {
		t.Fatalf("after setup: %+v", w)
	}
	v, err := s.Apply(ctx, Patch{Wizard: &WizardPatch{Step: ptr("library"), Skipped: []string{"tmdb", "nope"}}})
	if err != nil || v.Wizard.Step != "library" || len(v.Wizard.Skipped) != 1 {
		t.Fatalf("step: %v %+v", err, v.Wizard)
	}
	if _, err := s.Apply(ctx, Patch{Wizard: &WizardPatch{Completed: ptr(true)}}); err != nil || s.WizardPending(ctx) {
		t.Fatalf("complete: %v", err)
	}
}
