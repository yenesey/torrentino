package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"torrentino/common"
)

type contextKey int

const (
	requestIDKey contextKey = iota
	sessionKey
	externalPathKey
)

func requestIDFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(requestIDKey).(string); ok {
		return value
	}
	return ""
}

func requestID(r *http.Request) string {
	return requestIDFromContext(r.Context())
}

func sessionFromContext(r *http.Request) *Session {
	if value, ok := r.Context().Value(sessionKey).(*Session); ok {
		return value
	}
	return nil
}

func validRequestID(id string) bool {
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			continue
		}
		return false
	}
	return true
}

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" || len(id) > 64 || !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; " +
	"img-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; " +
	"base-uri 'none'; object-src 'none'"

func (s *Server) bodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Content-Security-Policy", contentSecurityPolicy)
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")
		if s.scheme(r) == "https" {
			header.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if !strings.Contains(r.URL.Path, "/static/") {
			header.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
	user   string
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(data)
	r.bytes += n
	return n, err
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		path := r.URL.Path
		mutating := r.Method != http.MethodGet
		interesting := mutating || status >= 400 ||
			strings.Contains(path, "/login") || strings.Contains(path, "/ui/")
		if !interesting {
			return
		}
		log.Printf("[Torrentino]: web: ip=%s user=%q method=%s path=%s status=%d dur=%s request_id=%s",
			s.clientIP(r), recorder.user, r.Method, path, status,
			time.Since(start).Round(time.Millisecond), requestID(r))
	})
}

func (s *Server) requireSession(next func(http.ResponseWriter, *http.Request, *Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := s.sessionFromRequest(r)
		if session == nil {
			s.clearCookie(w, r, sessionCookie)
			nextPath, ok := s.base.ValidNext(s.externalPath(r))
			if !ok {
				nextPath = s.base.URL("/find")
			}
			login := s.base.URL("/login") + "?next=" + url.QueryEscape(nextPath)
			if s.htmx(r) {
				w.Header().Set("HX-Redirect", login)
				w.WriteHeader(http.StatusOK)
				return
			}
			s.redirect(w, r, login)
			return
		}
		if recorder, ok := w.(*statusRecorder); ok {
			recorder.user = session.User
		}
		ctx := context.WithValue(r.Context(), sessionKey, session)
		next(w, r.WithContext(ctx), session)
	}
}

func (s *Server) requireCSRF(next func(http.ResponseWriter, *http.Request, *Session)) func(http.ResponseWriter, *http.Request, *Session) {
	return func(w http.ResponseWriter, r *http.Request, session *Session) {
		if err := r.ParseForm(); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			s.badRequest(w, r, "invalid form")
			return
		}
		token := r.Header.Get("X-CSRF-Token")
		if token == "" {
			token = r.PostFormValue("csrf")
		}
		if !tokenEqual(token, session.CSRF) {
			s.forbidden(w, r, "csrf")
			return
		}
		if !s.originOK(r) {
			s.forbidden(w, r, fmt.Sprintf("origin=%q host=%q scheme=%q",
				r.Header.Get("Origin"), r.Host, s.scheme(r)))
			return
		}
		next(w, r, session)
	}
}

// originOK compares the Origin header (when present) with the request host and
// the scheme derived from TLS or a trusted proxy.
func (s *Server) originOK(r *http.Request) bool {
	return originAllowed(r.Header.Get("Origin"), s.scheme(r), r.Host)
}

func (s *Server) scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if s.trustedRemote(r) {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			proto = strings.TrimSpace(strings.Split(proto, ",")[0])
			if proto == "https" || proto == "http" {
				return proto
			}
		}
	}
	if commonSecureCookies() {
		return "https"
	}
	return "http"
}

func commonSecureCookies() bool {
	return common.Settings.Web.SecureCookies
}

func (s *Server) trustedRemote(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, network := range s.trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *Server) clientIP(r *http.Request) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}
	if !s.trustedRemote(r) {
		return remoteHost
	}

	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			ip := net.ParseIP(candidate)
			if ip == nil {
				continue
			}
			if !s.ipTrusted(ip) {
				return ip.String()
			}
		}
		if len(parts) > 0 {
			if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
				return ip.String()
			}
		}
	}
	if realIP := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); realIP != nil {
		return realIP.String()
	}
	return remoteHost
}

func (s *Server) ipTrusted(ip net.IP) bool {
	for _, network := range s.trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *Server) htmx(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func (s *Server) fragment(r *http.Request) bool {
	return s.htmx(r) || r.URL.Query().Get("fragment") == "1"
}

func (s *Server) externalPath(r *http.Request) string {
	if value, ok := r.Context().Value(externalPathKey).(string); ok && value != "" {
		return value
	}
	return s.base.URL(r.URL.Path)
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, to string) {
	if s.htmx(r) {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func parseTrustedProxies(values []string) ([]*net.IPNet, error) {
	result := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: %w", value, err)
		}
		result = append(result, network)
	}
	return result, nil
}
