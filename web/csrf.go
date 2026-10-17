package web

import (
	"crypto/subtle"
	"net/url"
	"strings"
)

// tokenEqual compares two CSRF tokens in constant time.
func tokenEqual(a, b string) bool {
	if a == "" || b == "" || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// originAllowed checks a browser-supplied Origin header. A missing Origin is
// accepted because the CSRF token is still required (this also keeps curl and
// non-browser clients usable). A present Origin must be same-origin.
//
// "null" is an opaque origin: some privacy modes, extensions and sandboxed
// contexts send it for a normal same-site form. The caller has already
// verified the session/pre-auth CSRF token before reaching this function, so
// accepting null does not turn a cross-site form into a valid request: an
// attacker cannot read the token.
func originAllowed(origin, scheme, host string) bool {
	origin = strings.TrimSuffix(strings.TrimSpace(origin), ".")
	if origin == "" || origin == "null" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	if canonicalHost(u.Host) != canonicalHost(host) {
		return false
	}
	if scheme != "" && !strings.EqualFold(u.Scheme, scheme) {
		return false
	}
	return true
}

// canonicalHost normalises the differences browsers and proxies introduce:
// case, a trailing dot and the default port. A non-default port is kept.
func canonicalHost(host string) string {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if strings.HasSuffix(host, ":443") {
		host = strings.TrimSuffix(host, ":443")
	}
	if strings.HasSuffix(host, ":80") {
		host = strings.TrimSuffix(host, ":80")
	}
	return host
}
