package web

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
)

func parseTemplates() (*template.Template, error) {
	return template.New("torrentino").Funcs(template.FuncMap{
		"inc": func(value int) int { return value + 1 },
	}).ParseFS(templateFS, "*.html")
}

// render executes a template into a buffer first, so a template error cannot
// emit a half-written page after headers were sent.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, name string, data any) {
	page := name
	if name == "screen" && !s.fragment(r) {
		page = "layout"
	}
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, page, data); err != nil {
		log.Printf("[Torrentino]: web: template %q: %v request_id=%s", page, err, requestID(r))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// renderScreen renders the #screen fragment or the whole page. Callers must
// not hold screen.mu.
func (s *Server) renderScreen(w http.ResponseWriter, r *http.Request, session *Session, screen *Screen, status int) {
	screen.mu.Lock()
	view := s.buildScreenViewLocked(screen)
	screen.mu.Unlock()
	data := s.buildPageData(session, view)
	s.render(w, r, status, "screen", data)
}
