package flaresolverr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"torrentino/common"
)

func TestFetchUsesConfiguredEndpoint(t *testing.T) {
	old := common.Settings.FlareSolverr
	t.Cleanup(func() { common.Settings.FlareSolverr = old })

	var got request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"status":"ok","solution":{"status":200,"response":"<html><img src=\"/poster.jpg\"></html>"}}`))
	}))
	defer srv.Close()
	common.Settings.FlareSolverr.URL = srv.URL
	common.Settings.FlareSolverr.Timeout = "10s"

	html, err := Fetch(context.Background(), "https://tracker.example/details/1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "poster.jpg") {
		t.Errorf("html = %q", html)
	}
	if got.Cmd != "request.get" || got.URL != "https://tracker.example/details/1" || got.MaxTimeout <= 0 {
		t.Errorf("request = %+v", got)
	}
}

func TestFetchReportsChallengelessErrors(t *testing.T) {
	old := common.Settings.FlareSolverr
	t.Cleanup(func() { common.Settings.FlareSolverr = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","message":"challenge failed"}`))
	}))
	defer srv.Close()
	common.Settings.FlareSolverr.URL = srv.URL
	common.Settings.FlareSolverr.Timeout = "10s"

	if _, err := Fetch(context.Background(), "https://tracker.example/details/1"); err == nil {
		t.Fatal("Fetch succeeded for an error response")
	}
}
