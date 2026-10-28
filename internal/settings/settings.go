// Package settings holds the server settings that an administrator can
// edit from the UI (first-run assistant, Réglages › Serveur). A value given
// by the configuration file or an AETHER_* variable always wins and is
// read-only; otherwise the value stored in the database applies, else the
// default. Changes are applied at once when the component supports it
// (hooks), otherwise at the next start.
package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fs0ciety7000/aether-media/internal/config"
	"github.com/fs0ciety7000/aether-media/internal/secret"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

// Database keys (same names as the configuration keys they override).
const (
	keyName      = "server.name"
	keyLanguage  = "metadata.language"
	keyCountry   = "metadata.country"
	keyTMDB      = "metadata.tmdb_api_key"
	keyHWAccel   = "transcode.hwaccel"
	keyDevice    = "transcode.vaapi_device"
	keyMaxTrans  = "transcode.max_sessions"
	keyScanEvery = "library.scan_interval"
	keyWizard    = "setup.wizard"
)

// Value sources reported by the API.
const (
	SourceFile     = "file"
	SourceEnv      = "env"
	SourceDatabase = "database"
	SourceDefault  = "default"
)

// When a change takes effect.
const (
	AppliesLive    = "live"
	AppliesRestart = "restart"
)

// HWAccel is the hardware acceleration setting.
type HWAccel struct {
	Mode   string `json:"mode"`             // none | auto | vaapi | qsv | nvenc | videotoolbox
	Device string `json:"device,omitempty"` // VAAPI / QSV render node
}

// Values are effective settings.
type Values struct {
	ServerName    string
	Language      string
	Country       string
	TMDBKey       string
	HWAccel       HWAccel
	MaxTranscodes int
	ScanInterval  time.Duration // 0 = periodic rescans disabled
}

// Hooks apply live changes to running components (nil = not live).
type Hooks struct {
	TMDBKey       func(key string)
	Locale        func(language, country string)
	MaxTranscodes func(n int)
	ScanInterval  func(d time.Duration)
	// CheckTMDB validates a key against TMDB (tmdb.ErrInvalidKey when refused).
	CheckTMDB func(ctx context.Context, key string) error
}

// Service reads and writes settings.
type Service struct {
	store *store.Store
	box   *secret.Box
	cfg   config.Config

	mu    sync.Mutex
	hooks Hooks
	boot  Values // in use since start (restart-only values)
	cur   Values
}

// New loads the stored settings. cfg is the configuration as loaded (file,
// environment, defaults), before any database override.
func New(ctx context.Context, st *store.Store, box *secret.Box, cfg config.Config) (*Service, error) {
	s := &Service{store: st, box: box, cfg: cfg}
	v, err := s.effective(ctx)
	if err != nil {
		return nil, err
	}
	s.cur, s.boot = v, v
	return s, nil
}

// SetHooks registers the live appliers.
func (s *Service) SetHooks(h Hooks) {
	s.mu.Lock()
	s.hooks = h
	s.mu.Unlock()
}

// Config returns the configuration (paths, binaries) the server runs with.
func (s *Service) Config() config.Config { return s.cfg }

// Current returns the effective values.
func (s *Service) Current() Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// TMDBEnabled reports whether a TMDB key is in effect.
func (s *Service) TMDBEnabled() bool { return s.Current().TMDBKey != "" }

// Overlay returns cfg with the effective values (for components built at
// start-up).
func Overlay(cfg config.Config, v Values) config.Config {
	cfg.Server.Name = v.ServerName
	cfg.Metadata.Language = v.Language
	cfg.Metadata.Country = v.Country
	cfg.Metadata.TMDBAPIKey = v.TMDBKey
	cfg.Transcode.HWAccel = v.HWAccel.Mode
	cfg.Transcode.VAAPIDevice = v.HWAccel.Device
	cfg.Transcode.MaxSessions = v.MaxTranscodes
	cfg.Library.ScanInterval = config.Duration{Duration: v.ScanInterval} // 0: the watcher only follows file events
	return cfg
}

// ---- stored values ----

type row struct {
	value  json.RawMessage
	secret []byte
}

func (s *Service) rows(ctx context.Context) (map[string]row, error) {
	list, err := s.store.ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]row, len(list))
	for _, r := range list {
		out[r.Key] = row{value: json.RawMessage(r.Value), secret: r.Secret}
	}
	return out, nil
}

func (s *Service) origin(key string) string {
	switch s.cfg.Origin(key) {
	case "file":
		return SourceFile
	case "env":
		return SourceEnv
	}
	return ""
}

// sourceOf returns the source of key given the stored rows.
func (s *Service) sourceOf(rows map[string]row, keys ...string) string {
	for _, k := range keys {
		if o := s.origin(k); o != "" {
			return o
		}
	}
	for _, k := range keys {
		if _, ok := rows[k]; ok {
			return SourceDatabase
		}
	}
	return SourceDefault
}

func (s *Service) effective(ctx context.Context) (Values, error) {
	rows, err := s.rows(ctx)
	if err != nil {
		return Values{}, err
	}
	c := s.cfg
	v := Values{
		ServerName:    c.Server.Name,
		Language:      c.Metadata.Language,
		Country:       c.Metadata.Country,
		TMDBKey:       c.Metadata.TMDBAPIKey,
		HWAccel:       HWAccel{Mode: c.Transcode.HWAccel, Device: c.Transcode.VAAPIDevice},
		MaxTranscodes: c.Transcode.MaxSessions,
		ScanInterval:  c.Library.ScanInterval.Duration,
	}
	str := func(key string, dst *string) {
		if s.origin(key) != "" {
			return
		}
		if r, ok := rows[key]; ok {
			var x string
			if json.Unmarshal(r.value, &x) == nil {
				*dst = x
			}
		}
	}
	str(keyName, &v.ServerName)
	str(keyLanguage, &v.Language)
	str(keyCountry, &v.Country)
	if s.origin(keyTMDB) == "" {
		if r, ok := rows[keyTMDB]; ok && len(r.secret) > 0 {
			var key string
			if err := s.box.OpenJSON(r.secret, &key); err != nil {
				return v, fmt.Errorf("settings: tmdb key: %w", err)
			}
			v.TMDBKey = key
		}
	}
	if s.origin(keyHWAccel) == "" && s.origin(keyDevice) == "" {
		if r, ok := rows[keyHWAccel]; ok {
			var h HWAccel
			if json.Unmarshal(r.value, &h) == nil && h.Mode != "" {
				if h.Device == "" {
					h.Device = v.HWAccel.Device
				}
				v.HWAccel = h
			}
		}
	}
	if s.origin(keyMaxTrans) == "" {
		if r, ok := rows[keyMaxTrans]; ok {
			var n int
			if json.Unmarshal(r.value, &n) == nil && n > 0 {
				v.MaxTranscodes = n
			}
		}
	}
	if s.origin(keyScanEvery) == "" {
		if r, ok := rows[keyScanEvery]; ok {
			var minutes int
			if json.Unmarshal(r.value, &minutes) == nil && minutes >= 0 {
				v.ScanInterval = time.Duration(minutes) * time.Minute
			}
		}
	}
	if v.Language == "" {
		v.Language = "fr-FR"
	}
	return v, nil
}

// RegionOf returns the country part of a language tag ("fr-FR" → "FR").
func RegionOf(lang string) string {
	if _, r, ok := strings.Cut(lang, "-"); ok && len(r) == 2 {
		return strings.ToUpper(r)
	}
	return ""
}

// ---- API view ----

// Field is one setting as shown to administrators.
type Field[T any] struct {
	Value    T      `json:"value"`
	Source   string `json:"source"`   // file | env | database | default
	ReadOnly bool   `json:"readOnly"` // defined by the configuration
	Applies  string `json:"applies"`  // live | restart
	// RestartRequired: the saved value is not the one in use yet.
	RestartRequired bool `json:"restartRequired,omitempty"`
}

// SecretField is a credential: never returned in clear.
type SecretField struct {
	Set      bool   `json:"set"`
	Masked   string `json:"masked,omitempty"` // "…ab12"
	Source   string `json:"source"`
	ReadOnly bool   `json:"readOnly"`
	Applies  string `json:"applies"`
}

// Wizard is the first-run assistant progress.
type Wizard struct {
	Step      string   `json:"step"` // account | metadata | tmdb | library | summary | done
	Completed bool     `json:"completed"`
	Skipped   []string `json:"skipped,omitempty"`
}

// View is GET /settings.
type View struct {
	ServerName          Field[string]  `json:"serverName"`
	MetadataLanguage    Field[string]  `json:"metadataLanguage"`
	MetadataCountry     Field[string]  `json:"metadataCountry"`
	TMDBAPIKey          SecretField    `json:"tmdbApiKey"`
	HWAccel             Field[HWAccel] `json:"hwAccel"`
	MaxTranscodes       Field[int]     `json:"maxTranscodes"`
	ScanIntervalMinutes Field[int]     `json:"scanIntervalMinutes"`
	Wizard              Wizard         `json:"wizard"`
}

func mask(k string) string {
	if k == "" {
		return ""
	}
	if utf8.RuneCountInString(k) < 12 {
		return "…"
	}
	return "…" + k[len(k)-4:]
}

// View returns the settings with their sources.
func (s *Service) View(ctx context.Context) (View, error) {
	rows, err := s.rows(ctx)
	if err != nil {
		return View{}, err
	}
	s.mu.Lock()
	cur, boot := s.cur, s.boot
	s.mu.Unlock()
	ro := func(src string) bool { return src == SourceFile || src == SourceEnv }
	str := func(key, val, applies string) Field[string] {
		src := s.sourceOf(rows, key)
		return Field[string]{Value: val, Source: src, ReadOnly: ro(src), Applies: applies}
	}
	v := View{
		ServerName:       str(keyName, cur.ServerName, AppliesLive),
		MetadataLanguage: str(keyLanguage, cur.Language, AppliesLive),
		MetadataCountry:  str(keyCountry, cur.Country, AppliesLive),
	}
	tsrc := s.sourceOf(rows, keyTMDB)
	v.TMDBAPIKey = SecretField{Set: cur.TMDBKey != "", Masked: mask(cur.TMDBKey), Source: tsrc, ReadOnly: ro(tsrc), Applies: AppliesLive}
	if tsrc == SourceDatabase && cur.TMDBKey == "" {
		v.TMDBAPIKey.Source = SourceDefault
	}
	hsrc := s.sourceOf(rows, keyHWAccel, keyDevice)
	v.HWAccel = Field[HWAccel]{Value: cur.HWAccel, Source: hsrc, ReadOnly: ro(hsrc), Applies: AppliesRestart, RestartRequired: cur.HWAccel != boot.HWAccel}
	msrc := s.sourceOf(rows, keyMaxTrans)
	v.MaxTranscodes = Field[int]{Value: cur.MaxTranscodes, Source: msrc, ReadOnly: ro(msrc), Applies: AppliesLive}
	ssrc := s.sourceOf(rows, keyScanEvery)
	v.ScanIntervalMinutes = Field[int]{Value: int(cur.ScanInterval / time.Minute), Source: ssrc, ReadOnly: ro(ssrc), Applies: AppliesLive}
	if w, err := s.wizard(rows); err == nil {
		v.Wizard = w
	}
	return v, nil
}

// ---- updates ----

// Patch is PATCH /settings (absent fields are unchanged).
type Patch struct {
	ServerName          *string      `json:"serverName"`
	MetadataLanguage    *string      `json:"metadataLanguage"`
	MetadataCountry     *string      `json:"metadataCountry"` // "" = the language's region
	TMDBAPIKey          *string      `json:"tmdbApiKey"`      // "" removes the stored key
	HWAccel             *HWAccel     `json:"hwAccel"`
	MaxTranscodes       *int         `json:"maxTranscodes"`
	ScanIntervalMinutes *int         `json:"scanIntervalMinutes"` // 0 disables periodic rescans
	Wizard              *WizardPatch `json:"wizard"`
}

// WizardPatch updates the assistant progress.
type WizardPatch struct {
	Step      *string  `json:"step"`
	Completed *bool    `json:"completed"`
	Skipped   []string `json:"skipped"`
}

// ErrManaged: the field is defined by the configuration.
type ErrManaged struct{ Field string }

func (e *ErrManaged) Error() string { return e.Field + " est défini par la configuration du serveur" }

// ErrInvalid: the value is refused.
type ErrInvalid struct{ Field, Msg string }

func (e *ErrInvalid) Error() string { return e.Field + " : " + e.Msg }

var (
	langRe    = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)
	countryRe = regexp.MustCompile(`^[A-Z]{2}$`)
)

// HWModes are the accepted hardware acceleration modes.
var HWModes = []string{"none", "auto", "vaapi", "qsv", "nvenc", "videotoolbox"}

// WizardSteps are the assistant steps, in order.
var WizardSteps = []string{"account", "metadata", "tmdb", "library", "summary", "done"}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

type write struct {
	key    string
	value  any
	secret []byte
	del    bool
}

// Apply validates and stores p, applies live changes and returns the new view.
func (s *Service) Apply(ctx context.Context, p Patch) (View, error) {
	var ws []write
	managed := func(field string, keys ...string) error {
		for _, k := range keys {
			if s.origin(k) != "" {
				return &ErrManaged{Field: field}
			}
		}
		return nil
	}
	if p.ServerName != nil {
		if err := managed("serverName", keyName); err != nil {
			return View{}, err
		}
		n := strings.TrimSpace(*p.ServerName)
		if n == "" || utf8.RuneCountInString(n) > 64 {
			return View{}, &ErrInvalid{"serverName", "de 1 à 64 caractères"}
		}
		ws = append(ws, write{key: keyName, value: n})
	}
	if p.MetadataLanguage != nil {
		if err := managed("metadataLanguage", keyLanguage); err != nil {
			return View{}, err
		}
		if !langRe.MatchString(*p.MetadataLanguage) {
			return View{}, &ErrInvalid{"metadataLanguage", "code de langue attendu (ex. fr-FR)"}
		}
		ws = append(ws, write{key: keyLanguage, value: *p.MetadataLanguage})
	}
	if p.MetadataCountry != nil {
		if err := managed("metadataCountry", keyCountry); err != nil {
			return View{}, err
		}
		c := strings.ToUpper(strings.TrimSpace(*p.MetadataCountry))
		if c != "" && !countryRe.MatchString(c) {
			return View{}, &ErrInvalid{"metadataCountry", "code pays à deux lettres attendu (ex. FR)"}
		}
		ws = append(ws, write{key: keyCountry, value: c, del: c == ""})
	}
	if p.TMDBAPIKey != nil {
		if err := managed("tmdbApiKey", keyTMDB); err != nil {
			return View{}, err
		}
		k := strings.TrimSpace(*p.TMDBAPIKey)
		if len(k) > 1024 || strings.ContainsAny(k, " \t\r\n") {
			return View{}, &ErrInvalid{"tmdbApiKey", "clé invalide"}
		}
		w := write{key: keyTMDB, value: nil, del: k == ""}
		if k != "" {
			sealed, err := s.box.SealJSON(k)
			if err != nil {
				return View{}, err
			}
			w.secret = sealed
		}
		ws = append(ws, w)
	}
	if p.HWAccel != nil {
		if err := managed("hwAccel", keyHWAccel, keyDevice); err != nil {
			return View{}, err
		}
		h := HWAccel{Mode: strings.TrimSpace(p.HWAccel.Mode), Device: strings.TrimSpace(p.HWAccel.Device)}
		if !contains(HWModes, h.Mode) {
			return View{}, &ErrInvalid{"hwAccel", "mode inconnu (none, auto, vaapi, qsv, nvenc, videotoolbox)"}
		}
		if h.Device != "" && (!strings.HasPrefix(h.Device, "/") || len(h.Device) > 256) {
			return View{}, &ErrInvalid{"hwAccel", "chemin absolu attendu (ex. /dev/dri/renderD128)"}
		}
		if h.Device == "" {
			h.Device = s.cfg.Transcode.VAAPIDevice
		}
		ws = append(ws, write{key: keyHWAccel, value: h})
	}
	if p.MaxTranscodes != nil {
		if err := managed("maxTranscodes", keyMaxTrans); err != nil {
			return View{}, err
		}
		if n := *p.MaxTranscodes; n < 1 || n > 64 {
			return View{}, &ErrInvalid{"maxTranscodes", "de 1 à 64"}
		}
		ws = append(ws, write{key: keyMaxTrans, value: *p.MaxTranscodes})
	}
	if p.ScanIntervalMinutes != nil {
		if err := managed("scanIntervalMinutes", keyScanEvery); err != nil {
			return View{}, err
		}
		if m := *p.ScanIntervalMinutes; m != 0 && (m < 5 || m > 7*24*60) {
			return View{}, &ErrInvalid{"scanIntervalMinutes", "0 (désactivé) ou de 5 minutes à 7 jours"}
		}
		ws = append(ws, write{key: keyScanEvery, value: *p.ScanIntervalMinutes})
	}
	if p.Wizard != nil {
		w, err := s.Wizard(ctx)
		if err != nil {
			return View{}, err
		}
		if p.Wizard.Step != nil {
			if !contains(WizardSteps, *p.Wizard.Step) {
				return View{}, &ErrInvalid{"wizard", "étape inconnue"}
			}
			w.Step = *p.Wizard.Step
		}
		if p.Wizard.Completed != nil {
			w.Completed = *p.Wizard.Completed
			if w.Completed {
				w.Step = "done"
			}
		}
		if p.Wizard.Skipped != nil {
			w.Skipped = nil
			for _, st := range p.Wizard.Skipped {
				if contains(WizardSteps, st) && !contains(w.Skipped, st) {
					w.Skipped = append(w.Skipped, st)
				}
			}
		}
		ws = append(ws, write{key: keyWizard, value: w})
	}

	err := s.store.Tx(ctx, func(q *db.Queries) error {
		for _, w := range ws {
			if w.del {
				if err := q.DeleteSetting(ctx, w.key); err != nil {
					return err
				}
				continue
			}
			b, err := json.Marshal(w.value)
			if err != nil {
				return err
			}
			if err := q.UpsertSetting(ctx, db.UpsertSettingParams{Key: w.key, Value: string(b), Secret: w.secret}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return View{}, err
	}
	if err := s.reload(ctx); err != nil {
		return View{}, err
	}
	return s.View(ctx)
}

// reload recomputes the effective values and runs the live hooks.
func (s *Service) reload(ctx context.Context) error {
	v, err := s.effective(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	old, h := s.cur, s.hooks
	s.cur = v
	s.mu.Unlock()
	if v.TMDBKey != old.TMDBKey && h.TMDBKey != nil {
		h.TMDBKey(v.TMDBKey)
	}
	if (v.Language != old.Language || v.Country != old.Country) && h.Locale != nil {
		h.Locale(v.Language, v.Country)
	}
	if v.MaxTranscodes != old.MaxTranscodes && h.MaxTranscodes != nil {
		h.MaxTranscodes(v.MaxTranscodes)
	}
	if v.ScanInterval != old.ScanInterval && h.ScanInterval != nil {
		h.ScanInterval(v.ScanInterval)
	}
	return nil
}

// ---- TMDB key check ----

// ErrNoChecker: TMDB validation is not wired (tests).
var ErrNoChecker = errors.New("settings: tmdb check unavailable")

// CheckTMDB validates key, or the key in effect when key is empty.
func (s *Service) CheckTMDB(ctx context.Context, key string) error {
	s.mu.Lock()
	check := s.hooks.CheckTMDB
	if strings.TrimSpace(key) == "" {
		key = s.cur.TMDBKey
	}
	s.mu.Unlock()
	if check == nil {
		return ErrNoChecker
	}
	return check(ctx, key)
}

// ---- first-run assistant ----

func (s *Service) wizard(rows map[string]row) (Wizard, error) {
	r, ok := rows[keyWizard]
	if !ok {
		return Wizard{Step: "account"}, nil
	}
	var w Wizard
	if err := json.Unmarshal(r.value, &w); err != nil {
		return Wizard{Step: "account"}, nil
	}
	return w, nil
}

// Wizard returns the assistant progress.
func (s *Service) Wizard(ctx context.Context) (Wizard, error) {
	r, err := s.store.GetSetting(ctx, keyWizard)
	if errors.Is(err, sql.ErrNoRows) {
		return Wizard{Step: "account"}, nil
	}
	if err != nil {
		return Wizard{}, err
	}
	return s.wizard(map[string]row{keyWizard: {value: json.RawMessage(r.Value)}})
}

// WizardPending reports whether an administrator exists but the assistant
// was left before its end (it then resumes at the stored step).
func (s *Service) WizardPending(ctx context.Context) bool {
	r, err := s.store.GetSetting(ctx, keyWizard)
	if err != nil {
		return false
	}
	var w Wizard
	return json.Unmarshal([]byte(r.Value), &w) == nil && !w.Completed
}

// StartWizard records that the administrator account exists (POST /setup).
func (s *Service) StartWizard(ctx context.Context) error {
	b, _ := json.Marshal(Wizard{Step: "metadata"})
	return s.store.UpsertSetting(ctx, db.UpsertSettingParams{Key: keyWizard, Value: string(b)})
}
