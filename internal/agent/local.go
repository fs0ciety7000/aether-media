package agent

import (
	"bytes"
	"html/template"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// isLocalRequest reports whether r comes from this machine: loopback peer,
// loopback Host (defeats DNS rebinding from a web page) and no proxy
// headers (a local reverse proxy would make every client look local).
func isLocalRequest(r *http.Request) bool {
	ip := net.ParseIP(clientIP(r))
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Real-Ip", "Forwarded"} {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	hip := net.ParseIP(host)
	return hip != nil && hip.IsLoopback()
}

// localOnly serves h to local clients and answers 404 to everyone else.
func localOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLocalRequest(r) {
			writeError(w, http.StatusNotFound, "not_found", "point d'accès inconnu")
			return
		}
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		hd.Set("Cache-Control", "no-store")
		h(w, r)
	}
}

// listenURLs lists the URLs the agent can be reached at: every non-loopback
// IPv4 address of the machine when listening on all interfaces.
func (s *Server) listenURLs() []string {
	scheme := "http"
	if s.cfg.TLSCert != "" {
		scheme = "https"
	}
	host, port, err := net.SplitHostPort(s.cfg.Listen)
	if err != nil {
		return nil
	}
	if p := s.httpPort.Load(); p > 0 {
		port = strconv.Itoa(int(p))
	}
	mk := func(h string) string { return scheme + "://" + net.JoinHostPort(h, port) }
	if ip := net.ParseIP(host); host != "" && (ip == nil || !ip.IsUnspecified()) {
		return []string{mk(host)}
	}
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if v4 := ipn.IP.To4(); v4 != nil && !v4.IsLoopback() && !v4.IsLinkLocalUnicast() {
			out = append(out, mk(v4.String()))
		}
	}
	return out
}

type localServer struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	PairedAt time.Time `json:"pairedAt"`
}

type localStatus struct {
	UID              string        `json:"uid"`
	Name             string        `json:"name"`
	Hostname         string        `json:"hostname"`
	Version          string        `json:"version"`
	Addresses        []string      `json:"addresses"`
	PairingCode      string        `json:"pairingCode"`
	PairingExpiresAt time.Time     `json:"pairingExpiresAt"`
	DiscoveryPort    int           `json:"discoveryPort"` // 0: disabled
	FFprobe          bool          `json:"ffprobe"`
	Shares           []shareInfo   `json:"shares"`
	Servers          []localServer `json:"servers"`
}

func (s *Server) localStatus() localStatus {
	code, exp := s.pairing.current()
	st := localStatus{
		UID: s.uid, Name: s.name, Hostname: s.hostname, Version: s.version,
		Addresses:        s.listenURLs(),
		PairingCode:      code,
		PairingExpiresAt: exp.UTC(),
		FFprobe:          s.ffprobePath != "",
		Shares:           s.shareInfos(),
		Servers:          []localServer{},
	}
	if st.Addresses == nil {
		st.Addresses = []string{}
	}
	if s.cfg.DiscoveryPort > 0 {
		st.DiscoveryPort = s.cfg.DiscoveryPort
	}
	s.mu.RLock()
	for _, sv := range s.state.Servers {
		st.Servers = append(st.Servers, localServer{ID: sv.ID, Name: sv.Name, PairedAt: sv.PairedAt})
	}
	s.mu.RUnlock()
	return st
}

func (s *Server) handleLocalStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.localStatus())
}

type localPage struct {
	localStatus
	Remaining string
}

func (s *Server) handleLocalPage(w http.ResponseWriter, _ *http.Request) {
	st := s.localStatus()
	mins := int((time.Until(st.PairingExpiresAt) + time.Minute - time.Second) / time.Minute)
	rem := "moins d'une minute"
	if mins > 1 {
		rem = strconv.Itoa(mins) + " min"
	} else if mins == 1 {
		rem = "1 min"
	}
	var buf bytes.Buffer
	if err := localTmpl.Execute(&buf, localPage{localStatus: st, Remaining: rem}); err != nil {
		s.writeStatusError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

var localTmpl = template.Must(template.New("local").Funcs(template.FuncMap{
	"date": func(t time.Time) string { return t.Local().Format("02/01/2006 15:04") },
}).Parse(`<!doctype html>
<html lang="fr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="30">
<title>Agent Aether — {{.Name}}</title>
<style>
:root{--bg:#04121D;--panel:#0A2233;--line:#173A50;--text:#E6F1F5;--muted:#8AA6B5;--accent:#5EC4C0}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);font:15px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
main{max-width:760px;margin:0 auto;padding:32px 16px 48px}
header{display:flex;align-items:baseline;gap:12px;flex-wrap:wrap;margin-bottom:24px}
h1{margin:0;font-size:24px;font-weight:600}
h1 span{color:var(--accent)}
.ver{color:var(--muted);font-size:13px}
section{background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:18px 20px;margin-bottom:16px}
h2{margin:0 0 12px;font-size:13px;letter-spacing:.08em;text-transform:uppercase;color:var(--muted);font-weight:600}
.code{font:600 clamp(32px,9vw,48px)/1.1 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;letter-spacing:.12em;color:var(--accent);margin:4px 0 8px}
.hint{color:var(--muted);margin:0}
ul{list-style:none;margin:0;padding:0}
li{padding:8px 0;border-top:1px solid var(--line);display:flex;justify-content:space-between;gap:12px;flex-wrap:wrap}
li:first-child{border-top:0}
code,.mono{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:13px;word-break:break-all}
.tag{font-size:12px;color:var(--muted);border:1px solid var(--line);border-radius:999px;padding:0 8px}
.tag.on{color:var(--accent);border-color:var(--accent)}
.empty{color:var(--muted)}
footer{color:var(--muted);font-size:12px;text-align:center;margin-top:24px}
</style>
</head>
<body>
<main>
<header><h1><span>Aether</span> · {{.Name}}</h1><span class="ver">agent {{.Version}} · {{.Hostname}}</span></header>

<section>
<h2>Code d'appairage</h2>
<div class="code">{{.PairingCode}}</div>
<p class="hint">Valable encore {{.Remaining}}. Dans le serveur Aether, ajoutez un agent puis saisissez ce code. Il change après chaque appairage.</p>
</section>

<section>
<h2>Adresses</h2>
<ul>{{range .Addresses}}<li><span class="mono">{{.}}</span></li>{{else}}<li class="empty">Aucune adresse réseau détectée.</li>{{end}}</ul>
{{if .DiscoveryPort}}<p class="hint" style="margin-top:10px">Découverte automatique active (UDP {{.DiscoveryPort}}).</p>{{end}}
</section>

<section>
<h2>Partages</h2>
<ul>{{range .Shares}}<li><span><strong>{{.Name}}</strong> <code>{{.Path}}</code></span><span class="tag{{if .Managed}} on{{end}}">{{if .Unavailable}}indisponible{{else if .Managed}}interface{{else}}agent.toml{{end}}</span></li>{{else}}<li class="empty">Aucun partage : choisissez les dossiers depuis le serveur Aether après l'appairage.</li>{{end}}</ul>
</section>

<section>
<h2>Serveurs appairés</h2>
<ul>{{range .Servers}}<li><strong>{{.Name}}</strong><span class="ver">depuis le {{date .PairedAt}}</span></li>{{else}}<li class="empty">Aucun serveur appairé.</li>{{end}}</ul>
</section>

<footer>Page visible uniquement depuis cette machine · actualisée toutes les 30 s</footer>
</main>
</body>
</html>
`))
