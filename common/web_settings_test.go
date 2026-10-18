package common

import (
	"strings"
	"testing"
	"time"

	"torrentino/common/password"
)

func settingsWithWeb(t *testing.T, web string) string {
	t.Helper()
	return strings.Replace(validSettings, `"path":`, `"web": `+web+`, "path":`, 1)
}

func testHash(t *testing.T, pw string) string {
	t.Helper()
	hash, err := password.Hash(pw, password.MinIterations)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestWebDisabledDoesNotValidateWebFields(t *testing.T) {
	// Disabled web means the bot behaves exactly as before, even if the block
	// contains nonsense: no validation and no web server.
	body := settingsWithWeb(t, `{"enabled": false, "listen": "nonsense", "base-path": "//x", "page-size": 1}`)
	if err := LoadSettingsFrom(writeSettings(t, body)); err != nil {
		t.Fatalf("disabled web should not be validated: %v", err)
	}
}

func TestWebEnabledValidConfig(t *testing.T) {
	body := settingsWithWeb(t, `{
		"enabled": true,
		"listen": "127.0.0.1:8088",
		"base-path": "/torrentino/",
		"page-size": 20,
		"session": {"idle-timeout": "12h", "max-age": "168h"},
		"secure-cookies": true,
		"trusted-proxies": ["127.0.0.1/32"],
		"users": [{"name": "denis", "password-hash": "`+testHash(t, "pw")+`"}]
	}`)
	if err := LoadSettingsFrom(writeSettings(t, body)); err != nil {
		t.Fatalf("valid web config: %v", err)
	}
	if got := Settings.WebBasePath(); got != "/torrentino" {
		t.Errorf("WebBasePath() = %q, want normalized /torrentino", got)
	}
	if got := Settings.WebListen(); got != "127.0.0.1:8088" {
		t.Errorf("WebListen() = %q", got)
	}
	if got := Settings.WebPageSize(); got != 20 {
		t.Errorf("WebPageSize() = %d", got)
	}
	if got := Settings.WebSessionIdle(); got != 12*time.Hour {
		t.Errorf("WebSessionIdle() = %v", got)
	}
	if got := Settings.WebSessionMaxAge(); got != 168*time.Hour {
		t.Errorf("WebSessionMaxAge() = %v", got)
	}
}

func TestWebDefaults(t *testing.T) {
	body := settingsWithWeb(t, `{"enabled": true, "users": [{"name": "u", "password-hash": "`+testHash(t, "pw")+`"}]}`)
	if err := LoadSettingsFrom(writeSettings(t, body)); err != nil {
		t.Fatal(err)
	}
	if got := Settings.WebListen(); got != "127.0.0.1:8088" {
		t.Errorf("default listen = %q", got)
	}
	if got := Settings.WebBasePath(); got != "/" {
		t.Errorf("default base path = %q", got)
	}
	if got := Settings.WebPageSize(); got != 20 {
		t.Errorf("default page size = %d", got)
	}
	if got := Settings.WebSessionIdle(); got != 12*time.Hour {
		t.Errorf("default idle = %v", got)
	}
	if got := Settings.WebSessionMaxAge(); got != 168*time.Hour {
		t.Errorf("default max age = %v", got)
	}
}

func TestWebEnabledValidationProblems(t *testing.T) {
	hash := testHash(t, "pw")
	cases := []struct {
		name string
		web  string
		want string
	}{
		{"no users", `{"enabled": true}`, "web.users"},
		{"bad listen", `{"enabled": true, "listen": "nonsense", "users": [{"name": "u", "password-hash": "` + hash + `"}]}`, "web.listen"},
		{"bad port", `{"enabled": true, "listen": "127.0.0.1:70000", "users": [{"name": "u", "password-hash": "` + hash + `"}]}`, "web.listen"},
		{"bad base path", `{"enabled": true, "base-path": "//x", "users": [{"name": "u", "password-hash": "` + hash + `"}]}`, "web.base-path"},
		{"traversal base path", `{"enabled": true, "base-path": "/a/../b", "users": [{"name": "u", "password-hash": "` + hash + `"}]}`, "web.base-path"},
		{"page size", `{"enabled": true, "page-size": 1, "users": [{"name": "u", "password-hash": "` + hash + `"}]}`, "web.page-size"},
		{"bad hash", `{"enabled": true, "users": [{"name": "u", "password-hash": "nope"}]}`, "password-hash"},
		{"empty name", `{"enabled": true, "users": [{"name": "", "password-hash": "` + hash + `"}]}`, "web.users[0].name"},
		{"duplicate names", `{"enabled": true, "users": [{"name": "u", "password-hash": "` + hash + `"}, {"name": "U", "password-hash": "` + hash + `"}]}`, "duplicates"},
		{"tls half", `{"enabled": true, "tls": {"cert": "x"}, "users": [{"name": "u", "password-hash": "` + hash + `"}]}`, "tls.cert"},
		{"idle too long", `{"enabled": true, "session": {"idle-timeout": "200h", "max-age": "168h"}, "users": [{"name": "u", "password-hash": "` + hash + `"}]}`, "idle-timeout"},
		{"bad cidr", `{"enabled": true, "trusted-proxies": ["not-a-cidr"], "users": [{"name": "u", "password-hash": "` + hash + `"}]}`, "trusted-proxies"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := LoadSettingsFrom(writeSettings(t, settingsWithWeb(t, c.web)))
			if err == nil {
				t.Fatalf("want an error mentioning %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestNormalizeBasePath(t *testing.T) {
	valid := map[string]string{
		"":             "/",
		"/":            "/",
		"/torrentino":  "/torrentino",
		"/torrentino/": "/torrentino",
	}
	for input, want := range valid {
		got, err := NormalizeBasePath(input)
		if err != nil || got != want {
			t.Errorf("NormalizeBasePath(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"torrentino", "//x", "/a/../b", "/a//b", "/x?y", "/x#y", `/x\y`, "/x y"} {
		if got, err := NormalizeBasePath(input); err == nil {
			t.Errorf("NormalizeBasePath(%q) = %q, want an error", input, got)
		}
	}
}
