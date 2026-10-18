package web

import (
	"net/url"
	"strings"

	"torrentino/common"
)

// BasePath is the external URL prefix. In the main deployment it is "/" and
// all URLs stay as-is; in fallback mode it is e.g. "/torrentino" and every
// link, form action, redirect, static URL and cookie path includes it.
type BasePath struct {
	base string
}

func NewBasePath(raw string) (*BasePath, error) {
	base, err := common.NormalizeBasePath(raw)
	if err != nil {
		return nil, err
	}
	return &BasePath{base: base}, nil
}

// Base returns the normalized prefix without a trailing slash.
func (b *BasePath) Base() string {
	if b == nil || b.base == "" {
		return "/"
	}
	return b.base
}

// URL returns an external URL for an internal path ("/find", "/static/x").
func (b *BasePath) URL(path string) string {
	base := b.Base()
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if base == "/" {
		return path
	}
	if path == "/" {
		return base + "/"
	}
	return base + path
}

// Prefix returns the prefix in the form templates use: empty for the root
// deployment, "/torrentino" otherwise. Concatenating "{{.Base}}/login" then
// yields a correct URL in both modes.
func (b *BasePath) Prefix() string {
	if b.Base() == "/" {
		return ""
	}
	return b.Base()
}

// CookiePath is the Path attribute for cookies.
func (b *BasePath) CookiePath() string {
	return b.Base()
}

// Strip converts an external path into the path the internal mux expects. It
// also handles the bare prefix: "/torrentino" and "/torrentino/" both become
// "/", which the root handler redirects to /find.
func (b *BasePath) Strip(external string) (string, bool) {
	base := b.Base()
	if base == "/" {
		return external, true
	}
	if external == base {
		return "/", true
	}
	if external == base+"/" {
		return "/", true
	}
	if strings.HasPrefix(external, base+"/") {
		return external[len(base):], true
	}
	return "", false
}

// ValidNext accepts only a local path under the configured base. It rejects
// absolute URLs, protocol-relative URLs, backslashes and traversal segments.
func (b *BasePath) ValidNext(next string) (string, bool) {
	next = strings.TrimSpace(next)
	if next == "" || strings.ContainsAny(next, "\\\r\n") {
		return "", false
	}
	u, err := url.Parse(next)
	if err != nil || u.IsAbs() || u.Host != "" || u.Scheme != "" {
		return "", false
	}
	if !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "", false
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	base := b.Base()
	if base != "/" && u.Path != base && !strings.HasPrefix(u.Path, base+"/") {
		return "", false
	}
	// In the main (root) deployment the old fallback prefix is not a valid
	// destination: a login on the subdomain must not bounce into /torrentino.
	if base == "/" && (u.Path == "/torrentino" || strings.HasPrefix(u.Path, "/torrentino/")) {
		return "", false
	}
	if u.Path == "" {
		return "", false
	}
	if u.RawQuery != "" {
		return u.Path + "?" + u.RawQuery, true
	}
	return u.Path, true
}
