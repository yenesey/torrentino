package jackett

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"torrentino/common"
)

// pointJackettAt starts a fake Jackett and makes baseUrl() resolve to it.
func pointJackettAt(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	common.Settings.Jackett.Host = u.Hostname()
	common.Settings.Jackett.Port = port
	common.Settings.Jackett.APIKey = "key"
}

// The deadline used to be hardcoded in Query; it is now read from
// "jackett.timeout", so a short configured value must actually be applied.
func TestQueryUsesConfiguredTimeout(t *testing.T) {
	pointJackettAt(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte(`{"Results":[],"Indexers":[]}`))
	})
	common.Settings.Jackett.Timeout = "100ms"

	start := time.Now()
	_, err := Query("query", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("want a timeout error from Query")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error = %v, want it to mention the timeout", err)
	}
	if elapsed >= 500*time.Millisecond {
		t.Errorf("Query returned after %v, want it to respect the 100ms timeout", elapsed)
	}
}

func TestQueryParsesResults(t *testing.T) {
	pointJackettAt(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v2.0/") {
			t.Errorf("path = %q, want the Jackett API prefix", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("Query"); got != "test query" {
			t.Errorf("Query = %q, want %q", got, "test query")
		}
		if got := query["Tracker[]"]; len(got) != 1 || got[0] != "rutor" {
			t.Errorf("Tracker[] = %v, want [rutor]", got)
		}
		_, _ = w.Write([]byte(`{"Results":[{"Title":"result","Seeders":3}],"Indexers":[]}`))
	})
	common.Settings.Jackett.Timeout = ""

	results, err := Query("test query", []string{"rutor"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(*results) != 1 || (*results)[0].Title != "result" || (*results)[0].Seeders != 3 {
		t.Errorf("results = %+v", *results)
	}
}
