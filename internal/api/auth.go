package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fs0ciety7000/aether-media/internal/auth"
	"github.com/fs0ciety7000/aether-media/internal/store"
	"github.com/fs0ciety7000/aether-media/internal/store/db"
)

const sessionCookie = "aether_session"

// principal is the authenticated user of a request.
type principal struct {
	user      db.User
	libraries []int64 // nil = all
	tokenHash []byte
	sessionID int64
}

type ctxKey struct{}

func principalFrom(r *http.Request) *principal {
	p, _ := r.Context().Value(ctxKey{}).(*principal)
	return p
}

func isAdmin(r *http.Request) bool {
	p := principalFrom(r)
	return p != nil && p.user.IsAdmin == 1
}

// viewer turns the request's user into listing restrictions.
func viewer(r *http.Request) store.Viewer {
	p := principalFrom(r)
	if p == nil {
		return store.Viewer{LibraryIDs: []int64{}} // nobody: see nothing
	}
	v := store.Viewer{UserID: p.user.ID, LibraryIDs: p.libraries, BlockUnrated: p.user.BlockUnrated == 1}
	if p.user.MaxRatingAge.Valid {
		v.MaxAge, v.HasMaxAge = int(p.user.MaxRatingAge.Int64), true
	}
	return v
}

// publicAPI lists the endpoints reachable without a session.
var publicAPI = map[string]bool{
	"GET /api/v1/system/info": true,
	"POST /api/v1/auth/login": true,
	"POST /api/v1/setup":      true,
}

// authState holds login throttling and session touch bookkeeping.
type authState struct {
	limiter *auth.Limiter
	mu      sync.Mutex
	touched map[string]time.Time
}

func newAuthState() *authState {
	return &authState{limiter: auth.NewLimiter(10, 15*time.Minute), touched: map[string]time.Time{}}
}

// authenticate resolves the session of API requests and enforces CSRF
// protection for cookie-authenticated writes.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		token, fromCookie := bearer(r)
		if token != "" {
			if p := s.lookupSession(r.Context(), token); p != nil {
				if fromCookie && !safeMethod(r.Method) && !sameOrigin(r) {
					writeError(w, http.StatusForbidden, "bad_origin", "origine de la requête refusée")
					return
				}
				r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, p))
				next.ServeHTTP(w, r)
				return
			}
		}
		if publicAPI[r.Method+" "+r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusUnauthorized, "unauthenticated", "connexion requise")
	})
}

func bearer(r *http.Request) (token string, fromCookie bool) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):]), false
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value, true
	}
	return "", false
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// sameOrigin accepts requests without Origin (same-origin navigations and
// non-browser clients) and those whose Origin host matches the Host header.
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" || o == "null" {
		return o == ""
	}
	u, err := url.Parse(o)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func (s *Server) lookupSession(ctx context.Context, token string) *principal {
	hash := auth.HashToken(token)
	sess, err := s.store.GetSessionByToken(ctx, hash)
	if err != nil {
		return nil
	}
	u, err := s.store.GetUser(ctx, sess.UserID)
	if err != nil {
		return nil
	}
	p := &principal{user: u, tokenHash: hash, sessionID: sess.ID.Int64}
	if u.AllLibraries == 0 && u.IsAdmin == 0 {
		libs, err := s.store.ListUserLibraries(ctx, u.ID)
		if err != nil {
			return nil
		}
		p.libraries = libs
		if p.libraries == nil {
			p.libraries = []int64{}
		}
	}
	// Touch at most once a minute per session.
	key := string(hash)
	s.authState.mu.Lock()
	stale := time.Since(s.authState.touched[key]) > time.Minute
	if stale {
		s.authState.touched[key] = time.Now()
	}
	s.authState.mu.Unlock()
	if stale {
		_ = s.store.TouchSession(ctx, hash)
		_ = s.store.TouchUser(ctx, u.ID)
	}
	return p
}

// admin wraps handlers reserved to administrators.
func (s *Server) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(r) {
			writeError(w, http.StatusForbidden, "forbidden", "action réservée aux administrateurs")
			return
		}
		h(w, r)
	}
}

type userPrefs struct {
	AudioLanguage    string `json:"audioLanguage,omitempty"`
	SubtitleLanguage string `json:"subtitleLanguage,omitempty"`
	SubtitleMode     string `json:"subtitleMode,omitempty"`
}

type userDTO struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	IsAdmin      bool       `json:"isAdmin"`
	AllLibraries bool       `json:"allLibraries"`
	LibraryIDs   []int64    `json:"libraryIds"`
	MaxRatingAge *int64     `json:"maxRatingAge,omitempty"`
	BlockUnrated bool       `json:"blockUnrated"`
	Prefs        userPrefs  `json:"prefs"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastSeen     *time.Time `json:"lastSeen,omitempty"`
}

func (s *Server) userDTO(ctx context.Context, u db.User) userDTO {
	dto := userDTO{ID: u.ID, Name: u.Name, IsAdmin: u.IsAdmin == 1, AllLibraries: u.AllLibraries == 1,
		LibraryIDs: []int64{}, BlockUnrated: u.BlockUnrated == 1, CreatedAt: time.Unix(u.CreatedAt, 0).UTC()}
	if u.MaxRatingAge.Valid {
		v := u.MaxRatingAge.Int64
		dto.MaxRatingAge = &v
	}
	if u.LastSeen.Valid {
		t := time.Unix(u.LastSeen.Int64, 0).UTC()
		dto.LastSeen = &t
	}
	_ = json.Unmarshal([]byte(u.Prefs), &dto.Prefs)
	if libs, err := s.store.ListUserLibraries(ctx, u.ID); err == nil && libs != nil {
		dto.LibraryIDs = libs
	}
	return dto
}

type credentials struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Device   string `json:"device"`
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// setup: POST /api/v1/setup — creates the first administrator.
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	if n, err := s.store.CountUsers(r.Context()); err != nil || n > 0 {
		writeError(w, http.StatusConflict, "already_setup", "Aether est déjà configuré")
		return
	}
	u, ok := s.createUser(w, r, newUserRequest{Name: c.Name, Password: c.Password, IsAdmin: true, AllLibraries: ptrTrue()})
	if !ok {
		return
	}
	s.startSession(w, r, u, c.Device, http.StatusCreated)
}

func ptrTrue() *bool { t := true; return &t }

// login: POST /api/v1/auth/login
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.authState.limiter.Blocked(ip) {
		w.Header().Set("Retry-After", "900")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "trop de tentatives, réessayez dans quelques minutes")
		return
	}
	var c credentials
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	u, err := s.store.GetUserByName(r.Context(), strings.TrimSpace(c.Name))
	if err != nil {
		auth.VerifyDummy(c.Password)
		s.authState.limiter.Fail(ip)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "nom ou mot de passe incorrect")
		return
	}
	if ok, _ := auth.VerifyPassword(u.PasswordHash, c.Password); !ok {
		s.authState.limiter.Fail(ip)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "nom ou mot de passe incorrect")
		return
	}
	s.authState.limiter.Reset(ip)
	s.startSession(w, r, u, c.Device, http.StatusOK)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u db.User, device string, status int) {
	token, hash, err := auth.NewToken()
	if err != nil {
		s.internalError(w, err)
		return
	}
	if device == "" {
		device = r.UserAgent()
	}
	if len(device) > 200 {
		device = device[:200]
	}
	id := time.Now().UnixNano()
	if err := s.store.CreateSession(r.Context(), db.CreateSessionParams{
		ID: sql.NullInt64{Int64: id, Valid: true}, TokenHash: hash, UserID: u.ID, DeviceName: device, Client: "web",
	}); err != nil {
		s.internalError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", MaxAge: 400 * 24 * 3600,
	})
	writeJSON(w, status, map[string]any{"user": s.userDTO(r.Context(), u), "token": token})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if p := principalFrom(r); p != nil {
		_ = s.store.DeleteSession(r.Context(), p.tokenHash)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.userDTO(r.Context(), principalFrom(r).user))
}

type patchMeRequest struct {
	Prefs           *userPrefs `json:"prefs"`
	CurrentPassword string     `json:"currentPassword"`
	NewPassword     string     `json:"newPassword"`
}

func (s *Server) patchMe(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r)
	var req patchMeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	ctx := r.Context()
	if req.Prefs != nil {
		switch req.Prefs.SubtitleMode {
		case "", "auto", "always", "forced", "off":
		default:
			writeError(w, http.StatusBadRequest, "invalid_prefs", "mode de sous-titres inconnu")
			return
		}
		b, _ := json.Marshal(req.Prefs)
		if err := s.store.SetUserPrefs(ctx, db.SetUserPrefsParams{Prefs: string(b), ID: p.user.ID}); err != nil {
			s.internalError(w, err)
			return
		}
	}
	if req.NewPassword != "" {
		if ok, _ := auth.VerifyPassword(p.user.PasswordHash, req.CurrentPassword); !ok {
			writeError(w, http.StatusBadRequest, "wrong_password", "mot de passe actuel incorrect")
			return
		}
		if !s.setPassword(w, r, p.user.ID, req.NewPassword) {
			return
		}
		_ = s.store.DeleteOtherSessions(ctx, db.DeleteOtherSessionsParams{UserID: p.user.ID, TokenHash: p.tokenHash})
	}
	u, err := s.store.GetUser(ctx, p.user.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.userDTO(ctx, u))
}

func (s *Server) setPassword(w http.ResponseWriter, r *http.Request, userID int64, pw string) bool {
	if len([]rune(pw)) < auth.MinPasswordLen {
		writeError(w, http.StatusBadRequest, "weak_password", "le mot de passe doit faire au moins 8 caractères")
		return false
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		s.internalError(w, err)
		return false
	}
	if err := s.store.SetUserPassword(r.Context(), db.SetUserPasswordParams{PasswordHash: hash, ID: userID}); err != nil {
		s.internalError(w, err)
		return false
	}
	return true
}

type sessionDTO struct {
	ID        int64     `json:"id,string"` // 64-bit: a JSON string keeps JavaScript precise
	Device    string    `json:"device"`
	Client    string    `json:"client"`
	Current   bool      `json:"current"`
	LastSeen  time.Time `json:"lastSeen"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) mySessions(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r)
	list, err := s.store.ListUserSessions(r.Context(), p.user.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	out := []sessionDTO{}
	for _, se := range list {
		out = append(out, sessionDTO{ID: se.ID.Int64, Device: se.DeviceName, Client: se.Client,
			Current: string(se.TokenHash) == string(p.tokenHash), LastSeen: time.Unix(se.LastSeen, 0).UTC(), CreatedAt: time.Unix(se.CreatedAt, 0).UTC()})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("sid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "identifiant invalide")
		return
	}
	n, err := s.store.DeleteUserSession(r.Context(), db.DeleteUserSessionParams{UserID: principalFrom(r).user.ID, ID: sql.NullInt64{Int64: id, Valid: true}})
	if err != nil {
		s.internalError(w, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "not_found", "session introuvable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type newUserRequest struct {
	Name         string  `json:"name"`
	Password     string  `json:"password"`
	IsAdmin      bool    `json:"isAdmin"`
	AllLibraries *bool   `json:"allLibraries"`
	LibraryIDs   []int64 `json:"libraryIds"`
	MaxRatingAge *int64  `json:"maxRatingAge"`
	BlockUnrated bool    `json:"blockUnrated"`
}

func validAge(a *int64) bool {
	return a == nil || slices.Contains(auth.ParentalAges, int(*a))
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request, req newUserRequest) (db.User, bool) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 64 {
		writeError(w, http.StatusBadRequest, "invalid_name", "nom requis (64 caractères max)")
		return db.User{}, false
	}
	if len([]rune(req.Password)) < auth.MinPasswordLen {
		writeError(w, http.StatusBadRequest, "weak_password", "le mot de passe doit faire au moins 8 caractères")
		return db.User{}, false
	}
	if !validAge(req.MaxRatingAge) {
		writeError(w, http.StatusBadRequest, "invalid_age", "âge attendu : 0, 6, 10, 12, 16 ou 18")
		return db.User{}, false
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.internalError(w, err)
		return db.User{}, false
	}
	all := req.AllLibraries == nil || *req.AllLibraries
	var u db.User
	err = s.store.Tx(r.Context(), func(q *db.Queries) error {
		var err error
		u, err = q.CreateUser(r.Context(), db.CreateUserParams{Name: name, PasswordHash: hash, IsAdmin: b2i(req.IsAdmin),
			AllLibraries: b2i(all), MaxRatingAge: nullAge(req.MaxRatingAge), BlockUnrated: b2i(req.BlockUnrated)})
		if err != nil {
			return err
		}
		return setLibraries(r.Context(), q, u.ID, all, req.LibraryIDs)
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "name_taken", "ce nom est déjà utilisé")
			return u, false
		}
		s.internalError(w, err)
		return u, false
	}
	return u, true
}

func setLibraries(ctx context.Context, q *db.Queries, userID int64, all bool, ids []int64) error {
	if err := q.ClearUserLibraries(ctx, userID); err != nil {
		return err
	}
	if all {
		return nil
	}
	for _, id := range ids {
		if err := q.AddUserLibrary(ctx, db.AddUserLibraryParams{UserID: userID, LibraryID: id}); err != nil {
			return err
		}
	}
	return nil
}

func nullAge(a *int64) sql.NullInt64 {
	if a == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *a, Valid: true}
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	out := make([]userDTO, 0, len(users))
	for _, u := range users {
		out = append(out, s.userDTO(r.Context(), u))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) postUser(w http.ResponseWriter, r *http.Request) {
	var req newUserRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	u, ok := s.createUser(w, r, req)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, s.userDTO(r.Context(), u))
}

type patchUserRequest struct {
	Name         *string `json:"name"`
	Password     string  `json:"password"`
	IsAdmin      *bool   `json:"isAdmin"`
	AllLibraries *bool   `json:"allLibraries"`
	LibraryIDs   []int64 `json:"libraryIds"`
	MaxRatingAge *int64  `json:"maxRatingAge"`
	ClearAge     bool    `json:"-"`
	BlockUnrated *bool   `json:"blockUnrated"`
}

func (s *Server) loadUser(w http.ResponseWriter, r *http.Request) (db.User, bool) {
	id, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "identifiant invalide")
		return db.User{}, false
	}
	u, err := s.store.GetUser(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "utilisateur introuvable")
		return u, false
	}
	if err != nil {
		s.internalError(w, err)
		return u, false
	}
	return u, true
}

func (s *Server) patchUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.loadUser(w, r)
	if !ok {
		return
	}
	var raw map[string]json.RawMessage
	body := http.MaxBytesReader(w, r.Body, 1<<14)
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "corps JSON invalide")
		return
	}
	b, _ := json.Marshal(raw)
	var req patchUserRequest
	_ = json.Unmarshal(b, &req)
	// "maxRatingAge": null explicitly removes the parental limit.
	if v, present := raw["maxRatingAge"]; present && string(v) == "null" {
		req.ClearAge = true
	}
	ctx := r.Context()
	name := u.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 64 {
			writeError(w, http.StatusBadRequest, "invalid_name", "nom requis (64 caractères max)")
			return
		}
	}
	admin := u.IsAdmin == 1
	if req.IsAdmin != nil {
		admin = *req.IsAdmin
	}
	if !admin && u.IsAdmin == 1 {
		if n, _ := s.store.CountAdmins(ctx); n <= 1 {
			writeError(w, http.StatusConflict, "last_admin", "impossible de retirer le dernier administrateur")
			return
		}
	}
	all := u.AllLibraries == 1
	if req.AllLibraries != nil {
		all = *req.AllLibraries
	}
	age := u.MaxRatingAge
	if req.MaxRatingAge != nil {
		if !validAge(req.MaxRatingAge) {
			writeError(w, http.StatusBadRequest, "invalid_age", "âge attendu : 0, 6, 10, 12, 16 ou 18")
			return
		}
		age = nullAge(req.MaxRatingAge)
	} else if req.ClearAge {
		age = sql.NullInt64{}
	}
	block := u.BlockUnrated == 1
	if req.BlockUnrated != nil {
		block = *req.BlockUnrated
	}
	if req.Password != "" && !s.setPassword(w, r, u.ID, req.Password) {
		return
	}
	err := s.store.Tx(ctx, func(q *db.Queries) error {
		if err := q.UpdateUser(ctx, db.UpdateUserParams{Name: name, IsAdmin: b2i(admin), AllLibraries: b2i(all),
			MaxRatingAge: age, BlockUnrated: b2i(block), ID: u.ID}); err != nil {
			return err
		}
		if req.AllLibraries != nil || req.LibraryIDs != nil {
			return setLibraries(ctx, q, u.ID, all, req.LibraryIDs)
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "name_taken", "ce nom est déjà utilisé")
			return
		}
		s.internalError(w, err)
		return
	}
	if req.Password != "" {
		_ = s.store.DeleteUserSessions(ctx, u.ID) // a reset password logs the user out everywhere
	}
	u, _ = s.store.GetUser(ctx, u.ID)
	writeJSON(w, http.StatusOK, s.userDTO(ctx, u))
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.loadUser(w, r)
	if !ok {
		return
	}
	if u.IsAdmin == 1 {
		if n, _ := s.store.CountAdmins(r.Context()); n <= 1 {
			writeError(w, http.StatusConflict, "last_admin", "impossible de supprimer le dernier administrateur")
			return
		}
	}
	if err := s.store.DeleteUser(r.Context(), u.ID); err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
