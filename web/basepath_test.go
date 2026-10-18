package web

import "testing"

func TestBasePathURLs(t *testing.T) {
	root, err := NewBasePath("/")
	if err != nil {
		t.Fatal(err)
	}
	if root.Base() != "/" || root.Prefix() != "" || root.CookiePath() != "/" {
		t.Errorf("root base = %q prefix = %q cookie = %q", root.Base(), root.Prefix(), root.CookiePath())
	}
	for input, want := range map[string]string{
		"":      "/",
		"/":     "/",
		"/find": "/find",
		"find":  "/find",
	} {
		if got := root.URL(input); got != want {
			t.Errorf("root.URL(%q) = %q, want %q", input, got, want)
		}
	}

	prefix, err := NewBasePath("/torrentino/")
	if err != nil {
		t.Fatal(err)
	}
	if prefix.Base() != "/torrentino" || prefix.Prefix() != "/torrentino" || prefix.CookiePath() != "/torrentino" {
		t.Errorf("prefix base = %q prefix = %q cookie = %q", prefix.Base(), prefix.Prefix(), prefix.CookiePath())
	}
	for input, want := range map[string]string{
		"":      "/torrentino/",
		"/":     "/torrentino/",
		"/find": "/torrentino/find",
		"find":  "/torrentino/find",
	} {
		if got := prefix.URL(input); got != want {
			t.Errorf("prefix.URL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBasePathStrip(t *testing.T) {
	root, _ := NewBasePath("/")
	for input, want := range map[string]string{
		"/":         "/",
		"/find":     "/find",
		"/static/x": "/static/x",
	} {
		got, ok := root.Strip(input)
		if !ok || got != want {
			t.Errorf("root.Strip(%q) = %q, %v", input, got, ok)
		}
	}

	prefix, _ := NewBasePath("/torrentino")
	for input, want := range map[string]string{
		"/torrentino":          "/",
		"/torrentino/":         "/",
		"/torrentino/find":     "/find",
		"/torrentino/static/x": "/static/x",
	} {
		got, ok := prefix.Strip(input)
		if !ok || got != want {
			t.Errorf("prefix.Strip(%q) = %q, %v", input, got, ok)
		}
	}
	for _, input := range []string{"/", "/find", "/torrentinoX", "/other/torrentino"} {
		if _, ok := prefix.Strip(input); ok {
			t.Errorf("prefix.Strip(%q) unexpectedly succeeded", input)
		}
	}
}

func TestValidNext(t *testing.T) {
	root, _ := NewBasePath("/")
	valid := []string{"/find", "/downloads", "/find?x=1"}
	for _, next := range valid {
		if got, ok := root.ValidNext(next); !ok || got != next {
			t.Errorf("root.ValidNext(%q) = %q, %v", next, got, ok)
		}
	}
	invalid := []string{
		"", "//evil", "https://evil", "javascript:alert(1)", "/a/../b",
		`/a\b`, "/torrentino", "/torrentino/downloads",
	}
	for _, next := range invalid {
		if _, ok := root.ValidNext(next); ok {
			t.Errorf("root.ValidNext(%q) unexpectedly succeeded", next)
		}
	}

	prefix, _ := NewBasePath("/torrentino")
	if got, ok := prefix.ValidNext("/torrentino/downloads"); !ok || got != "/torrentino/downloads" {
		t.Errorf("prefix.ValidNext = %q, %v", got, ok)
	}
	for _, next := range []string{"/downloads", "/torrentinoX/find", "//evil/torrentino"} {
		if _, ok := prefix.ValidNext(next); ok {
			t.Errorf("prefix.ValidNext(%q) unexpectedly succeeded", next)
		}
	}
}
