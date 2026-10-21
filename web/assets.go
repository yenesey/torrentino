package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed templates/*.html static
var contentFS embed.FS

var (
	templateFS, _ = fs.Sub(contentFS, "templates")
	staticFS, _   = fs.Sub(contentFS, "static")
)

// staticVersion is a content hash used in ?v= URLs so an updated binary
// invalidates browser caches without a manual version bump.
func staticVersion() string {
	hash := sha256.New()
	_ = fs.WalkDir(staticFS, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(staticFS, name)
		if err != nil {
			return nil
		}
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write(data)
		return nil
	})
	return hex.EncodeToString(hash.Sum(nil))[:12]
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/static/")
	if name == "" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
		http.NotFound(w, r)
		return
	}
	name = path.Clean(name)
	data, err := fs.ReadFile(staticFS, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch path.Ext(name) {
	case ".ttf":
		w.Header().Set("Content-Type", "font/ttf")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	default:
		if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}
