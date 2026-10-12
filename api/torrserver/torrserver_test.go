package torrserver

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"torrentino/common"
)

// pointAt redirects the package at a local test server. The endpoint is read
// from common.Settings on every call, so this needs no rebuild of the package.
func pointAt(t *testing.T, srv *httptest.Server) {
	t.Helper()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split test server host: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}

	common.Settings.Torrserver.Host = host
	common.Settings.Torrserver.Port = port
}

// captureAdd runs Add against a stub server and returns the raw request body.
// The body travels through a channel so the test reads it with a proper
// happens-before edge instead of sharing memory with the handler goroutine.
func captureAdd(t *testing.T, title string) []byte {
	t.Helper()

	bodies := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- body
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	pointAt(t, srv)

	if err := Add("magnet:?xt=urn:btih:REAL", title, "http://tracker/poster.jpg"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	return <-bodies
}

// A title with JSON metacharacters must survive the round trip intact: before
// this was fixed the body was built by string concatenation and came out
// malformed, so Add failed with a request error.
func TestAddEscapesJSONMetacharacters(t *testing.T) {
	const dirty = "Movie \"Best\" Cut \\ 2024\nsecond line\ttab"

	raw := captureAdd(t, dirty)

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("request body is not valid JSON: %v\nbody: %s", err, raw)
	}
	if got["title"] != dirty {
		t.Errorf("title = %q, want %q", got["title"], dirty)
	}
	if got["link"] != "magnet:?xt=urn:btih:REAL" {
		t.Errorf("link = %v", got["link"])
	}
	if got["poster"] != "http://tracker/poster.jpg" {
		t.Errorf("poster = %v", got["poster"])
	}
	if got["save_to_db"] != true {
		t.Errorf("save_to_db = %v, want true", got["save_to_db"])
	}
}

// The title is authored by whoever uploaded the torrent, so it is untrusted
// input. It must never be able to rewrite the request: no injected action, no
// swapped link, no extra fields.
func TestAddIsNotInjectableThroughTitle(t *testing.T) {
	const evil = `x", "link" : "magnet:?xt=EVIL", "action" : "rem", "hash" : "VICTIM`

	raw := captureAdd(t, evil)

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("request body is not valid JSON: %v\nbody: %s", err, raw)
	}
	if got["action"] != "add" {
		t.Errorf("action = %v, want add", got["action"])
	}
	if got["link"] != "magnet:?xt=urn:btih:REAL" {
		t.Errorf("link was rewritten: %v", got["link"])
	}
	if _, injected := got["hash"]; injected {
		t.Errorf("injected field reached the request: %v", got["hash"])
	}
	if got["title"] != evil {
		t.Errorf("title = %q, want it kept verbatim", got["title"])
	}
}

func TestListParsesItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[` +
			`{"Title":"good","Torrent_Size":10,"Hash":"h1","Data":"{\"torrserver\":{}}"},` +
			`{"Title":"odd","Torrent_Size":20,"Hash":"h2","Data":"not json at all"}` +
			`]`))
	}))
	defer srv.Close()
	pointAt(t, srv)

	list, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(*list) != 2 {
		t.Fatalf("got %d items, want 2", len(*list))
	}
	if (*list)[0].Title != "good" || (*list)[0].Hash != "h1" {
		t.Errorf("unexpected first item: %+v", (*list)[0])
	}
	// "data" is documented as arbitrary custom content, so one entry that is
	// not JSON must not fail the whole listing.
	if (*list)[1].Title != "odd" {
		t.Errorf("unexpected second item: %+v", (*list)[1])
	}
}

func TestAddReportsServerMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unsupported action", http.StatusBadRequest)
	}))
	defer srv.Close()
	pointAt(t, srv)

	err := Add("magnet:?xt=urn:btih:REAL", "title", "")
	if err == nil {
		t.Fatal("want an error for a non-200 response")
	}
	if !strings.Contains(err.Error(), "unsupported action") {
		t.Errorf("error = %v, want it to carry the server message", err)
	}
}
