package main

import (
	"context"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/config"
	"github.com/fs0ciety7000/aether-media/internal/library"
	"github.com/fs0ciety7000/aether-media/internal/metadata/refresh"
	"github.com/fs0ciety7000/aether-media/internal/metadata/tmdb"
	"github.com/fs0ciety7000/aether-media/internal/secret"
	"github.com/fs0ciety7000/aether-media/internal/settings"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/stream"
)

// loadSettings reads the settings edited from the admin UI and returns cfg
// with the effective values (configuration > database > defaults), which
// the components built at start-up use.
func loadSettings(ctx context.Context, st *store.Store, box *secret.Box, cfg config.Config) (*settings.Service, config.Config, error) {
	set, err := settings.New(ctx, st, box, cfg)
	if err != nil {
		return nil, cfg, err
	}
	return set, settings.Overlay(cfg, set.Current()), nil
}

// bindSettings applies later changes to the running components. Hardware
// acceleration has no hook: it takes effect at the next start.
func bindSettings(set *settings.Service, tm *tmdb.Provider, rf *refresh.Refresher, streams *stream.Manager, w *library.Watcher) {
	set.SetHooks(settings.Hooks{
		TMDBKey: tm.SetAPIKey,
		Locale: func(lang, country string) {
			tm.SetLocale(lang, country)
			rf.SetLanguage(lang)
		},
		MaxTranscodes: func(n int) {
			if streams != nil {
				streams.SetMaxTranscodes(n)
			}
		},
		ScanInterval: func(d time.Duration) {
			if w != nil {
				w.SetInterval(d)
			}
		},
		CheckTMDB: tm.CheckKey,
	})
}
