package web

import (
	"crypto/sha256"
	"testing"
	"time"
)

func TestSessionStoreIdleAndAbsoluteExpiry(t *testing.T) {
	store := NewSessionStore(time.Hour, 2*time.Hour)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	token, session, err := store.Create("denis")
	if err != nil {
		t.Fatal(err)
	}
	if session.User != "denis" || session.CSRF == "" {
		t.Errorf("session = %+v", session)
	}
	if got, ok := store.Get(token); !ok || got != session {
		t.Fatal("session not found right after creation")
	}

	now = now.Add(30 * time.Minute)
	if _, ok := store.Get(token); !ok {
		t.Fatal("session expired before the idle timeout")
	}

	now = now.Add(61 * time.Minute)
	if _, ok := store.Get(token); ok {
		t.Error("session did not expire after the idle timeout")
	}
}

func TestSessionStoreAbsoluteExpiry(t *testing.T) {
	store := NewSessionStore(10*time.Hour, 2*time.Hour)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	token, _, err := store.Create("denis")
	if err != nil {
		t.Fatal(err)
	}
	// Keep touching the session so the idle timeout never fires.
	for i := 0; i < 3; i++ {
		now = now.Add(30 * time.Minute)
		if _, ok := store.Get(token); !ok {
			t.Fatal("session expired during the absolute test")
		}
	}
	now = now.Add(time.Hour)
	if _, ok := store.Get(token); ok {
		t.Error("session outlived the absolute timeout")
	}
}

func TestSessionStoreDelete(t *testing.T) {
	store := NewSessionStore(time.Hour, 2*time.Hour)
	token, _, err := store.Create("denis")
	if err != nil {
		t.Fatal(err)
	}
	store.Delete(token)
	if _, ok := store.Get(token); ok {
		t.Error("deleted session was found")
	}
	store.Delete("missing") // must not panic
}

func TestSessionStoreEvictsOldestAtLimit(t *testing.T) {
	store := NewSessionStore(time.Hour, 2*time.Hour)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	var first string
	for i := 0; i < maxSessions+1; i++ {
		token, _, err := store.Create("denis")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = token
		}
		now = now.Add(time.Second)
	}
	if got := store.Len(); got != maxSessions {
		t.Errorf("Len = %d, want %d", got, maxSessions)
	}
	if _, ok := store.Get(first); ok {
		t.Error("the oldest session was not evicted")
	}
}

func TestSessionStoreHashesTokens(t *testing.T) {
	store := NewSessionStore(time.Hour, 2*time.Hour)
	token, _, err := store.Create("denis")
	if err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte(token))
	store.mu.RLock()
	_, ok := store.sessions[key]
	store.mu.RUnlock()
	if !ok {
		t.Error("session is not indexed by the SHA-256 of its token")
	}
	if token == "" {
		t.Error("empty token")
	}
}

func TestCSRFTokenEqual(t *testing.T) {
	if !tokenEqual("abc", "abc") {
		t.Error("equal tokens were rejected")
	}
	for _, pair := range [][2]string{{"", ""}, {"", "x"}, {"abc", "abd"}, {"abc", "abcd"}} {
		if tokenEqual(pair[0], pair[1]) {
			t.Errorf("tokenEqual(%q, %q) = true", pair[0], pair[1])
		}
	}
}

func TestOriginAllowed(t *testing.T) {
	cases := []struct {
		origin, scheme, host string
		want                 bool
	}{
		{"", "https", "example.com", true},
		{"https://example.com", "https", "example.com", true},
		{"https://example.com:443", "https", "example.com:443", true},
		{"https://example.com:443", "https", "example.com", true},
		{"https://example.com.", "https", "example.com", true},
		{"https://example.com", "https", "example.com:443", true},
		{"https://evil.com", "https", "example.com", false},
		{"http://example.com", "https", "example.com", false},
		{"null", "https", "example.com", true}, // opaque origin, CSRF token still required
		{"not a url", "https", "example.com", false},
		{"https://example.com", "", "example.com", true},
	}
	for _, c := range cases {
		if got := originAllowed(c.origin, c.scheme, c.host); got != c.want {
			t.Errorf("originAllowed(%q, %q, %q) = %v, want %v", c.origin, c.scheme, c.host, got, c.want)
		}
	}
}
