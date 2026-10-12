package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validSettings = `{
  "jackett":      { "host": "127.0.0.1", "port": 9117, "api-key": "key", "indexers": ["rutor"] },
  "transmission": { "host": "127.0.0.1", "port": 9091 },
  "torrserver":   { "host": "127.0.0.1", "port": 8090 },
  "telegram-api-token": "123456:token",
  "users-list": [1, 2],
  "path": { "default": "/tmp/default", "movie": "/tmp/movie", "series": "/tmp/series" }
}`

func writeSettings(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), settingsFileName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The point of LoadSettingsFrom is that loading a config is an ordinary,
// testable function: importing this package must not terminate the process.
func TestLoadSettingsFromValidFile(t *testing.T) {
	if err := LoadSettingsFrom(writeSettings(t, validSettings)); err != nil {
		t.Fatalf("LoadSettingsFrom: %v", err)
	}
	if Settings.TelegramAPIToken != "123456:token" {
		t.Errorf("TelegramAPIToken = %q", Settings.TelegramAPIToken)
	}
	if Settings.Jackett.Host != "127.0.0.1" || Settings.Jackett.Port != 9117 {
		t.Errorf("jackett = %+v", Settings.Jackett)
	}
	if Settings.Transmission.Port != 9091 {
		t.Errorf("transmission = %+v", Settings.Transmission)
	}
	if Settings.Torrserver.Port != 8090 {
		t.Errorf("torrserver = %+v", Settings.Torrserver)
	}
	if Settings.Path.Movie != "/tmp/movie" {
		t.Errorf("path.movie = %q, want the value from the path object", Settings.Path.Movie)
	}
	if len(Settings.UsersList) != 2 {
		t.Errorf("users-list = %v", Settings.UsersList)
	}
}

func TestLoadSettingsFromMissingFile(t *testing.T) {
	err := LoadSettingsFrom(filepath.Join(t.TempDir(), "absent.json"))
	if err == nil {
		t.Fatal("want an error for a missing config")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want it to say the file was not found", err)
	}
}

func TestLoadSettingsFromBrokenJSON(t *testing.T) {
	err := LoadSettingsFrom(writeSettings(t, `{"telegram-api-token": "x",}`))
	if err == nil {
		t.Fatal("want an error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "cannot parse") {
		t.Errorf("error = %v, want a parse error", err)
	}
}

func TestLoadSettingsFromIncompleteFile(t *testing.T) {
	err := LoadSettingsFrom(writeSettings(t, `{
	  "jackett": { "host": "", "port": 0, "api-key": "" },
	  "telegram-api-token": ""
	}`))
	if err == nil {
		t.Fatal("want an error for an incomplete config")
	}
	for _, want := range []string{"telegram-api-token", "jackett.host", "jackett.port", "jackett.api-key"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to report %q", err, want)
		}
	}
}
