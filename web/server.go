package web

import (
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"torrentino/common"
	"torrentino/core"
)

// Server is the optional HTTP interface. It lives in the same process as the
// Telegram bot, so both share common.Settings and the core service.
type Server struct {
	service       *core.Service
	base          *BasePath
	store         *SessionStore
	users         map[string]common.WebUser
	limiter       *loginLimiter
	searchLimiter *windowLimiter

	templates     *template.Template
	staticVersion string
	dummyHash     string
	trusted       []*net.IPNet

	mux        *http.ServeMux
	httpServer *http.Server
	listener   net.Listener

	downloads *downloadsCache

	ctx    context.Context
	cancel context.CancelFunc
}

// NewServer validates the web configuration again (settings validation already
// did) and prepares the HTTP handler. It does not bind a port; call Start for
// that.
func NewServer(service *core.Service) (*Server, error) {
	if service == nil {
		service = core.Default
	}
	base, err := NewBasePath(common.Settings.WebBasePath())
	if err != nil {
		return nil, err
	}

	users := make(map[string]common.WebUser, len(common.Settings.Web.Users))
	for _, user := range common.Settings.Web.Users {
		name := strings.TrimSpace(user.Name)
		users[strings.ToLower(name)] = common.WebUser{Name: name, PasswordHash: user.PasswordHash}
	}

	trusted, err := parseTrustedProxies(common.Settings.Web.TrustedProxies)
	if err != nil {
		return nil, err
	}

	s := &Server{
		service:       service,
		base:          base,
		store:         NewSessionStore(common.Settings.WebSessionIdle(), common.Settings.WebSessionMaxAge()),
		users:         users,
		limiter:       newLoginLimiter(loginMaxFailures, loginWindow),
		searchLimiter: newWindowLimiter(10, time.Minute),
		staticVersion: staticVersion(),
		dummyHash:     dummyPasswordHash,
		trusted:       trusted,
		downloads:     newDownloadsCache(2 * time.Second),
	}
	s.templates, err = parseTemplates()
	if err != nil {
		return nil, err
	}
	s.mux = http.NewServeMux()
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /login", s.handleLoginGET)
	s.mux.HandleFunc("POST /login", s.handleLoginPOST)
	s.mux.HandleFunc("GET /static/", s.handleStatic)
	s.mux.HandleFunc("GET /favicon.ico", s.handleFavicon)

	s.mux.HandleFunc("GET /", s.requireSession(func(w http.ResponseWriter, r *http.Request, _ *Session) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		s.redirect(w, r, s.base.URL("/find"))
	}))
	s.mux.HandleFunc("GET /find", s.requireSession(s.handleFind))
	s.mux.HandleFunc("POST /find/search", s.requireSession(s.requireCSRF(s.handleFindSearch)))
	s.mux.HandleFunc("GET /find/torrent", s.requireSession(s.handleFindTorrent))
	s.mux.HandleFunc("GET /downloads", s.requireSession(s.handleDownloads))
	s.mux.HandleFunc("GET /torrserver", s.requireSession(s.handleTorrserver))
	s.mux.HandleFunc("POST /ui/{kind}/command", s.requireSession(s.requireCSRF(s.handleCommand)))
	s.mux.HandleFunc("POST /logout", s.requireSession(s.requireCSRF(s.handleLogout)))
}

// Handler returns the external handler, including base-path stripping and
// middleware. It is exported for httptest.
func (s *Server) Handler() http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		external := r.URL.RequestURI()
		internal, ok := s.base.Strip(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(context.WithValue(r.Context(), externalPathKey, external))
		u := *r.URL
		u.Path = internal
		r2.URL = &u
		s.mux.ServeHTTP(w, r2)
	})
	return s.requestID(s.securityHeaders(s.accessLog(s.bodyLimit(inner))))
}

// Start binds the configured address and serves in the background. A bind
// error is returned to the caller (it is a fatal configuration error).
func (s *Server) Start() error {
	address := common.Settings.WebListen()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}
	s.listener = listener

	s.httpServer = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.gcLoop()

	useTLS := strings.TrimSpace(common.Settings.Web.TLS.Cert) != ""
	if useTLS {
		// Load the certificate synchronously: a bad path or key is a
		// configuration error and must stop startup, not just log from the
		// serving goroutine after the process is already up.
		if _, err := tls.LoadX509KeyPair(common.Settings.Web.TLS.Cert, common.Settings.Web.TLS.Key); err != nil {
			_ = listener.Close()
			return fmt.Errorf("load TLS certificate: %w", err)
		}
	}
	go func() {
		var err error
		if useTLS {
			err = s.httpServer.ServeTLS(listener,
				common.Settings.Web.TLS.Cert, common.Settings.Web.TLS.Key)
		} else {
			err = s.httpServer.Serve(listener)
		}
		if err != nil && err != http.ErrServerClosed {
			log.Printf("[Torrentino]: web server stopped: %v", err)
		}
	}()
	log.Printf("[Torrentino]: web listening on %s%s (tls=%v)", address, s.base.Base(), useTLS)
	return nil
}

func (s *Server) gcLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.store.GC()
		case <-s.ctx.Done():
			return
		}
	}
}

// Shutdown stops accepting requests and waits up to the context deadline for
// in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the bound address after Start.
func (s *Server) Addr() string {
	if s.listener == nil {
		return common.Settings.WebListen()
	}
	return s.listener.Addr().String()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
