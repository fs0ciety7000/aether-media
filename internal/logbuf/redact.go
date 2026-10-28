package logbuf

import (
	"regexp"
	"strings"
)

const masked = "[masqué]"

// sensitiveKey reports attribute names whose value is always masked.
func sensitiveKey(k string) bool {
	k = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(k, "_", ""), "-", ""))
	for _, s := range []string{"token", "secret", "password", "passphrase", "apikey", "authorization", "cookie", "privatekey", "clientsecret"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

var redactions = []struct {
	re   *regexp.Regexp
	repl string
}{
	// Credentials in query strings or form bodies: api_key=…, token=…
	{regexp.MustCompile(`(?i)\b((?:api_?key|api-key|access_token|refresh_token|id_token|token|client_secret|secret|password|passwd|code|key|sig|signature)=)[^&\s"']+`), "${1}" + masked},
	// HTTP authorisation headers.
	{regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[A-Za-z0-9._~+/=-]{8,}`), "${1} " + masked},
	// JSON fields carrying secrets.
	{regexp.MustCompile(`(?i)("(?:access_token|refresh_token|client_secret|clientSecret|password|token|api_key|apiKey|device_code|deviceCode|privateKey|passphrase)"\s*:\s*)"[^"]*"`), `${1}"` + masked + `"`},
	// Signed internal proxy URLs given to FFmpeg: /internal/vfs/{source}/{expiry}/{signature}/…
	{regexp.MustCompile(`(/internal/vfs/\d+/\d+/)[^/\s"']+`), "${1}" + masked},
	// JWTs (TMDB v4 read access tokens, OAuth tokens).
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+`), masked},
	// user:password@ in URLs.
	{regexp.MustCompile(`(://[^/\s:@]+:)[^/\s@]+@`), "${1}" + masked + "@"},
}

// Redact masks credentials found in s.
func Redact(s string) string {
	if s == "" {
		return s
	}
	for _, r := range redactions {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}
