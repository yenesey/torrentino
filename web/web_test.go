package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hekmon/transmissionrpc/v2"

	"torrentino/api/jackett"
	"torrentino/api/torrserver"
	"torrentino/common"
	"torrentino/common/password"
	"torrentino/core"
)

type fakeJackett struct {
	results  []jackett.Result
	queryErr error
	hash     string
	hashErr  error
}

func (f *fakeJackett) Query(ctx context.Context, query string, indexers []string) ([]jackett.Result, error) {
	return f.results, f.queryErr
}
func (f *fakeJackett) GetInfoHash(ctx context.Context, link string) (string, error) {
	return f.hash, f.hashErr
}

type fakeTransmission struct {
	torrents []transmissionrpc.Torrent
	listErr  error
	added    []string
	deleted  []int64
}

func (f *fakeTransmission) List(ctx context.Context) ([]transmissionrpc.Torrent, error) {
	return f.torrents, f.listErr
}
func (f *fakeTransmission) Add(ctx context.Context, link string, dir string) error {
	f.added = append(f.added, link+"|"+dir)
	return nil
}
func (f *fakeTransmission) Delete(ctx context.Context, id int64) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeTransmission) Start(ctx context.Context, id int64) error { return nil }
func (f *fakeTransmission) Pause(ctx context.Context, id int64) error { return nil }

type fakeTorrserver struct {
	items   []torrserver.TSListItem
	removed []string
	listErr error
}

func (f *fakeTorrserver) List(ctx context.Context) ([]torrserver.TSListItem, error) {
	return f.items, f.listErr
}
func (f *fakeTorrserver) Add(ctx context.Context, link, title, poster string) error { return nil }
func (f *fakeTorrserver) Delete(ctx context.Context, hash string) error {
	f.removed = append(f.removed, hash)
	return nil
}

func ptr[T any](value T) *T { return &value }

type testEnv struct {
	server    *Server
	jackett   *fakeJackett
	tr        *fakeTransmission
	ts        *fakeTorrserver
	http      *httptest.Server
	client    *http.Client
	baseURL   *url.URL
	basePath  string
	loginName string
	password  string
}

func newTestEnv(t *testing.T, basePath string) *testEnv {
	t.Helper()
	old := common.Settings
	t.Cleanup(func() { common.Settings = old })

	hash, err := password.Hash("secret", password.MinIterations)
	if err != nil {
		t.Fatal(err)
	}
	common.Settings.Web.Enabled = true
	common.Settings.Web.Listen = "127.0.0.1:0"
	common.Settings.Web.BasePath = basePath
	common.Settings.Web.PageSize = 5
	common.Settings.Web.Users = []common.WebUser{{Name: "denis", PasswordHash: hash}}
	common.Settings.Web.Session.IdleTimeout = "12h"
	common.Settings.Web.Session.MaxAge = "168h"
	common.Settings.Path.Default = t.TempDir()
	common.Settings.Path.Movie = ""
	common.Settings.Path.Series = ""

	tr := &fakeTransmission{}
	ts := &fakeTorrserver{}
	jackett := &fakeJackett{}
	service := &core.Service{Jackett: jackett, Transmission: tr, Torrserver: ts}

	server, err := NewServer(service)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	baseURL, err := url.Parse(httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &testEnv{
		server: server, jackett: jackett, tr: tr, ts: ts,
		http: httpServer, client: client, baseURL: baseURL,
		basePath: basePath, loginName: "denis", password: "secret",
	}
}

func (e *testEnv) prefix() string {
	if e.basePath == "/" {
		return ""
	}
	return e.basePath
}

func (e *testEnv) url(path string) string {
	return e.http.URL + e.prefix() + path
}

func (e *testEnv) cookie(name string) string {
	lookup := *e.baseURL
	lookup.Path = e.prefix() + "/"
	for _, cookie := range e.client.Jar.Cookies(&lookup) {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

type response struct {
	status int
	body   string
	header http.Header
}

func (e *testEnv) do(t *testing.T, method, path string, form url.Values, csrf string, htmx bool) response {
	return e.doPath(t, method, e.prefix()+path, form, csrf, htmx)
}

// doPath takes the complete external path, without adding the prefix.
func (e *testEnv) doPath(t *testing.T, method, path string, form url.Values, csrf string, htmx bool) response {
	t.Helper()
	var body *strings.Reader
	if form == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, e.http.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	res, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return response{status: res.StatusCode, body: string(data), header: res.Header}
}

func (e *testEnv) login(t *testing.T) *Session {
	t.Helper()
	res := e.do(t, http.MethodGet, "/login", nil, "", false)
	if res.status != http.StatusOK {
		t.Fatalf("GET login = %d", res.status)
	}
	loginToken := e.cookie(loginCookie)
	if loginToken == "" {
		t.Fatal("pre-auth cookie is missing")
	}
	res = e.do(t, http.MethodPost, "/login", url.Values{
		"username": {e.loginName},
		"password": {e.password},
		"csrf":     {loginToken},
		"next":     {e.prefix() + "/find"},
	}, "", false)
	if res.status != http.StatusSeeOther {
		t.Fatalf("POST login = %d, body=%s", res.status, res.body)
	}
	token := e.cookie(sessionCookie)
	if token == "" {
		t.Fatal("session cookie is missing")
	}
	session, ok := e.server.store.Get(token)
	if !ok {
		t.Fatal("session is not stored")
	}
	return session
}

func (e *testEnv) csrf(t *testing.T) string {
	t.Helper()
	session := e.login(t)
	return session.CSRF
}

func TestProtectedRouteRedirectsToLogin(t *testing.T) {
	env := newTestEnv(t, "/")
	res := env.do(t, http.MethodGet, "/find", nil, "", false)
	if res.status != http.StatusSeeOther {
		t.Fatalf("GET /find = %d, want 303", res.status)
	}
	if got, want := res.header.Get("Location"), "/login?next=%2Ffind"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestHealthAndStaticArePublic(t *testing.T) {
	env := newTestEnv(t, "/")
	if res := env.do(t, http.MethodGet, "/healthz", nil, "", false); res.status != http.StatusOK || res.body != `{"status":"ok"}` {
		t.Errorf("healthz = %d %q", res.status, res.body)
	}
	res := env.do(t, http.MethodGet, "/static/app.css", nil, "", false)
	if res.status != http.StatusOK {
		t.Errorf("static = %d", res.status)
	}
	if cache := res.header.Get("Cache-Control"); !strings.Contains(cache, "immutable") {
		t.Errorf("static Cache-Control = %q", cache)
	}
	if res := env.do(t, http.MethodGet, "/favicon.ico", nil, "", false); res.status != http.StatusNoContent {
		t.Errorf("favicon = %d", res.status)
	}
}

func TestLoginCookiesAndLogout(t *testing.T) {
	env := newTestEnv(t, "/")
	get := env.do(t, http.MethodGet, "/login", nil, "", false)
	if get.status != http.StatusOK || !strings.Contains(get.body, "Log in") {
		t.Fatalf("login page = %d, body=%s", get.status, get.body)
	}
	loginToken := env.cookie(loginCookie)
	if loginToken == "" {
		t.Fatal("pre-auth cookie is missing")
	}
	bad := env.do(t, http.MethodPost, "/login", url.Values{
		"username": {"denis"}, "password": {"wrong"}, "csrf": {loginToken},
	}, "", false)
	if bad.status != http.StatusOK || !strings.Contains(bad.body, "Invalid username or password") {
		t.Errorf("bad login = %d, body=%s", bad.status, bad.body)
	}
	if env.cookie(sessionCookie) != "" {
		t.Error("a session was created for a wrong password")
	}

	good := env.do(t, http.MethodPost, "/login", url.Values{
		"username": {"denis"}, "password": {"secret"}, "csrf": {loginToken},
		"next": {"/find"},
	}, "", false)
	if good.status != http.StatusSeeOther || good.header.Get("Location") != "/find" {
		t.Fatalf("login = %d, Location=%q", good.status, good.header.Get("Location"))
	}
	sessionCookieHeader := ""
	for _, header := range good.header.Values("Set-Cookie") {
		if strings.HasPrefix(header, sessionCookie+"=") {
			sessionCookieHeader = header
		}
	}
	if sessionCookieHeader == "" {
		t.Fatal("session Set-Cookie is missing")
	}
	for _, part := range []string{"Path=/", "HttpOnly", "SameSite=Lax", "Max-Age=604800"} {
		if !strings.Contains(sessionCookieHeader, part) {
			t.Errorf("session cookie %q does not contain %q", sessionCookieHeader, part)
		}
	}
	if strings.Contains(strings.ToLower(sessionCookieHeader), "domain=") {
		t.Errorf("session cookie must be host-only: %q", sessionCookieHeader)
	}

	page := env.do(t, http.MethodGet, "/find", nil, "", false)
	if page.status != http.StatusOK || !strings.Contains(page.body, "Torrentino") {
		t.Fatalf("GET /find = %d, body=%s", page.status, page.body)
	}
	session, ok := env.server.store.Get(env.cookie(sessionCookie))
	if !ok {
		t.Fatal("session is missing")
	}
	if session == nil {
		t.Fatal("session is missing")
	}
	logout := env.do(t, http.MethodPost, "/logout", url.Values{"csrf": {session.CSRF}}, "", false)
	if logout.status != http.StatusSeeOther {
		t.Errorf("logout = %d", logout.status)
	}
	if res := env.do(t, http.MethodGet, "/find", nil, "", false); res.status != http.StatusSeeOther {
		t.Errorf("GET /find after logout = %d, want redirect", res.status)
	}
}

func TestLoginRateLimit(t *testing.T) {
	env := newTestEnv(t, "/")
	env.do(t, http.MethodGet, "/login", nil, "", false)
	token := env.cookie(loginCookie)
	for i := 0; i < loginMaxFailures; i++ {
		res := env.do(t, http.MethodPost, "/login", url.Values{
			"username": {"denis"}, "password": {"wrong"}, "csrf": {token},
		}, "", false)
		if res.status != http.StatusOK {
			t.Fatalf("attempt %d = %d", i, res.status)
		}
	}
	res := env.do(t, http.MethodPost, "/login", url.Values{
		"username": {"denis"}, "password": {"secret"}, "csrf": {token},
	}, "", false)
	if res.status != http.StatusTooManyRequests {
		t.Errorf("rate limited login = %d, want 429", res.status)
	}
	if res.header.Get("Retry-After") == "" {
		t.Error("Retry-After header is missing")
	}
}

func TestFindSearchCommandsAndActions(t *testing.T) {
	env := newTestEnv(t, "/")
	torrents := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("d4:infod4:name4:teste e"))
	}))
	defer torrents.Close()

	env.jackett.results = []jackett.Result{{
		Title:     `<script>alert(1)</script> The Matrix`,
		InfoHash:  "abc",
		Size:      1024,
		TrackerId: "rutor",
		Seeders:   5,
		Peers:     2,
		Link:      torrents.URL + "/a.torrent",
		Details:   "javascript:alert(1)",
	}}
	csrf := env.csrf(t)

	search := env.do(t, http.MethodPost, "/find/search", url.Values{"q": {"matrix"}}, csrf, true)
	if search.status != http.StatusOK {
		t.Fatalf("search = %d, body=%s", search.status, search.body)
	}
	if got := search.header.Get("HX-Push-Url"); got != "/find" {
		t.Errorf("HX-Push-Url = %q", got)
	}
	if !strings.Contains(search.body, "results: 1-1 of 1") {
		t.Errorf("search body lacks the header: %s", search.body)
	}
	if strings.Contains(search.body, "<script>alert(1)</script>") {
		t.Error("tracker title was not escaped")
	}
	if !strings.Contains(search.body, "&lt;script&gt;") {
		t.Error("escaped tracker title is missing")
	}
	if strings.Contains(search.body, `href="javascript:`) {
		t.Error("a javascript: link reached the page")
	}

	selectRes := env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"select:0"}}, csrf, true)
	if selectRes.status != http.StatusOK || !strings.Contains(selectRes.body, `class="row selected"`) {
		t.Fatalf("select = %d, body=%s", selectRes.status, selectRes.body)
	}
	if !strings.Contains(selectRes.body, "download:movie") || !strings.Contains(selectRes.body, ".torrent") {
		t.Errorf("action panel is missing expected actions: %s", selectRes.body)
	}

	controls := env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"controls:toggle"}}, csrf, true)
	if controls.status != http.StatusOK || !strings.Contains(controls.body, "Sort") {
		t.Errorf("controls = %d", controls.status)
	}
	sorted := env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"sort:Size"}}, csrf, true)
	if sorted.status != http.StatusOK || !strings.Contains(sorted.body, "▲") {
		t.Errorf("sort did not render the ascending arrow: %s", sorted.body)
	}
	filtered := env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"filter:TrackerId/rutor"}}, csrf, true)
	if filtered.status != http.StatusOK || !strings.Contains(filtered.body, "✓ rutor") {
		t.Errorf("filter did not render the check mark: %s", filtered.body)
	}

	if err := func() error {
		_, err := env.server.service.Search(context.Background(), "matrix")
		return err
	}(); err != nil {
		t.Fatal(err)
	}
	selectRes = env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"select:0"}}, csrf, true)
	if selectRes.status != http.StatusOK {
		t.Fatalf("select after filter = %d", selectRes.status)
	}
	action := env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"action:download"}}, csrf, true)
	if action.status != http.StatusOK {
		t.Fatalf("download = %d, body=%s", action.status, action.body)
	}
	if len(env.tr.added) != 1 || !strings.HasPrefix(env.tr.added[0], torrents.URL+"/a.torrent|") {
		t.Errorf("added = %v", env.tr.added)
	}
	if !strings.Contains(action.body, "Added to Transmission") || !strings.Contains(action.body, "in Transmission") {
		t.Errorf("download response = %s", action.body)
	}

	torrent := env.do(t, http.MethodGet, "/find/torrent?item=0", nil, "", false)
	if torrent.status != http.StatusOK {
		t.Fatalf("torrent = %d, body=%s", torrent.status, torrent.body)
	}
	if got := torrent.header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-bittorrent") {
		t.Errorf("Content-Type = %q", got)
	}
	if got := torrent.header.Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Errorf("Content-Disposition = %q", got)
	}
	if !strings.HasPrefix(torrent.body, "d") {
		t.Errorf("body = %q", torrent.body)
	}
}

func TestCSRFAndOrigin(t *testing.T) {
	env := newTestEnv(t, "/")
	csrf := env.csrf(t)
	missing := env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"controls:toggle"}}, "", true)
	if missing.status != http.StatusForbidden {
		t.Errorf("missing CSRF = %d, want 403", missing.status)
	}
	bad := env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"controls:toggle"}}, "wrong", true)
	if bad.status != http.StatusForbidden {
		t.Errorf("wrong CSRF = %d, want 403", bad.status)
	}

	req, _ := http.NewRequest(http.MethodPost, env.url("/ui/find/command"), strings.NewReader("cmd=controls%3Atoggle"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Origin", "https://evil.example")
	res, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("bad Origin = %d, want 403", res.StatusCode)
	}
}

func TestSearchErrorIsShownInBanner(t *testing.T) {
	env := newTestEnv(t, "/")
	csrf := env.csrf(t)
	env.jackett.queryErr = io.ErrUnexpectedEOF
	res := env.do(t, http.MethodPost, "/find/search", url.Values{"q": {"matrix"}}, csrf, true)
	if res.status != http.StatusOK {
		t.Fatalf("search error = %d", res.status)
	}
	if !strings.Contains(res.body, "role=\"alert\"") {
		t.Errorf("no error banner: %s", res.body)
	}
}

func TestDownloadsScreenAndDelete(t *testing.T) {
	env := newTestEnv(t, "/")
	now := time.Now()
	env.tr.torrents = []transmissionrpc.Torrent{{
		ID:             ptr[int64](7),
		Name:           ptr("Some.Movie"),
		HashString:     ptr("abc"),
		DownloadDir:    ptr(common.Settings.Path.Default),
		DownloadedEver: ptr[int64](1024),
		PercentDone:    ptr(1.0),
		UploadRatio:    ptr(2.0),
		AddedDate:      &now,
		Status:         ptr(transmissionrpc.TorrentStatusSeed),
		Files:          []*transmissionrpc.TorrentFile{{Name: "movie.mkv"}},
	}}
	csrf := env.csrf(t)

	page := env.do(t, http.MethodGet, "/downloads", nil, "", false)
	if page.status != http.StatusOK {
		t.Fatalf("downloads = %d", page.status)
	}
	for _, want := range []string{"Some.Movie", "seeding", "downloaded /", "volume:"} {
		if !strings.Contains(page.body, want) {
			t.Errorf("downloads body lacks %q", want)
		}
	}

	selectRes := env.do(t, http.MethodPost, "/ui/downloads/command", url.Values{"cmd": {"select:0"}}, csrf, true)
	if selectRes.status != http.StatusOK || !strings.Contains(selectRes.body, `value="action:delete"`) {
		t.Fatalf("select = %d, body=%s", selectRes.status, selectRes.body)
	}
	deleted := env.do(t, http.MethodPost, "/ui/downloads/command", url.Values{"cmd": {"action:delete"}}, csrf, true)
	if deleted.status != http.StatusOK {
		t.Fatalf("delete = %d, body=%s", deleted.status, deleted.body)
	}
	if len(env.tr.deleted) != 1 || env.tr.deleted[0] != 7 {
		t.Errorf("deleted = %v", env.tr.deleted)
	}
	if !strings.Contains(deleted.body, "the list is empty") {
		t.Errorf("deleted item is still rendered: %s", deleted.body)
	}
	if !strings.Contains(deleted.body, "Deleted") {
		t.Errorf("success banner is missing: %s", deleted.body)
	}
}

func TestDownloadsSelectionSurvivesPoll(t *testing.T) {
	env := newTestEnv(t, "/")
	now := time.Now()
	makeTorrent := func(id int64, name string) transmissionrpc.Torrent {
		return transmissionrpc.Torrent{
			ID:             &id,
			Name:           &name,
			HashString:     ptr("hash"),
			DownloadDir:    ptr(common.Settings.Path.Default),
			DownloadedEver: ptr[int64](1),
			PercentDone:    ptr(0.5),
			UploadRatio:    ptr(0.0),
			AddedDate:      &now,
			Status:         ptr(transmissionrpc.TorrentStatusDownload),
			Files:          []*transmissionrpc.TorrentFile{{Name: "file.mkv"}},
		}
	}
	env.tr.torrents = []transmissionrpc.Torrent{makeTorrent(1, "first"), makeTorrent(2, "second")}
	csrf := env.csrf(t)
	env.do(t, http.MethodGet, "/downloads", nil, "", false)
	selectRes := env.do(t, http.MethodPost, "/ui/downloads/command", url.Values{"cmd": {"select:0"}}, csrf, true)
	if selectRes.status != http.StatusOK {
		t.Fatalf("select = %d", selectRes.status)
	}
	session, ok := env.server.store.Get(env.cookie(sessionCookie))
	if !ok {
		t.Fatal("session is missing")
	}
	screen := env.server.screenFor(session, KindDownloads)
	screen.mu.Lock()
	key := screen.SelectedKey
	beforeID := int64(0)
	if item, ok := screen.View.ItemAt(screen.View.Selected).(*core.DownloadItem); ok {
		beforeID = *item.ID
	}
	screen.mu.Unlock()
	if key == "" || beforeID == 0 {
		t.Fatal("selection is empty")
	}

	// Reverse the list and force a fresh snapshot: the selection must follow
	// the item, not the index.
	env.tr.torrents = []transmissionrpc.Torrent{makeTorrent(2, "second"), makeTorrent(1, "first")}
	env.server.downloads.Invalidate()
	poll := env.do(t, http.MethodGet, "/downloads?fragment=1", nil, "", true)
	if poll.status != http.StatusOK {
		t.Fatalf("poll = %d", poll.status)
	}
	screen.mu.Lock()
	selectedKey := screen.SelectedKey
	selectedID := int64(0)
	if item, ok := screen.View.ItemAt(screen.View.Selected).(*core.DownloadItem); ok {
		selectedID = *item.ID
	}
	screen.mu.Unlock()
	if selectedKey != key {
		t.Errorf("selection key changed from %q to %q", key, selectedKey)
	}
	if selectedID != beforeID {
		t.Errorf("selected ID = %d, want %d", selectedID, beforeID)
	}
}

func TestDownloadsDiskDelete(t *testing.T) {
	env := newTestEnv(t, "/")
	target := common.Settings.Path.Default + "/diskfile.mkv"
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	csrf := env.csrf(t)
	env.do(t, http.MethodGet, "/downloads", nil, "", false)
	session, ok := env.server.store.Get(env.cookie(sessionCookie))
	if !ok {
		t.Fatal("session is missing")
	}
	screen := env.server.screenFor(session, KindDownloads)
	screen.mu.Lock()
	index := -1
	for i := 0; i < screen.View.Len(); i++ {
		if item, ok := screen.View.Item(i).(*core.DownloadItem); ok && item.Status == "unknown" {
			index = i
		}
	}
	screen.mu.Unlock()
	if index < 0 {
		t.Fatal("disk item is missing")
	}
	selectRes := env.do(t, http.MethodPost, "/ui/downloads/command", url.Values{"cmd": {"select:" + itoa(index)}}, csrf, true)
	if selectRes.status != http.StatusOK || !strings.Contains(selectRes.body, "Delete files now") {
		t.Fatalf("select disk item = %d, body=%s", selectRes.status, selectRes.body)
	}
	deleted := env.do(t, http.MethodPost, "/ui/downloads/command", url.Values{"cmd": {"action:delete"}}, csrf, true)
	if deleted.status != http.StatusOK {
		t.Fatalf("disk delete = %d, body=%s", deleted.status, deleted.body)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("file still exists: %v", err)
	}
	if len(env.tr.deleted) != 0 {
		t.Errorf("Transmission delete was called for a disk-only item: %v", env.tr.deleted)
	}
}

func TestTorrserverScreenAndDelete(t *testing.T) {
	env := newTestEnv(t, "/")
	env.ts.items = []torrserver.TSListItem{
		{Title: "First", Torrent_Size: 20, Hash: "h1"},
		{Title: "Second", Torrent_Size: 10, Hash: "h2"},
	}
	csrf := env.csrf(t)
	page := env.do(t, http.MethodGet, "/torrserver", nil, "", false)
	if page.status != http.StatusOK || !strings.Contains(page.body, "First") || !strings.Contains(page.body, "Second") {
		t.Fatalf("torrserver = %d, body=%s", page.status, page.body)
	}
	selectRes := env.do(t, http.MethodPost, "/ui/torrserver/command", url.Values{"cmd": {"select:0"}}, csrf, true)
	if selectRes.status != http.StatusOK || !strings.Contains(selectRes.body, `value="action:delete"`) {
		t.Fatalf("select = %d, body=%s", selectRes.status, selectRes.body)
	}
	deleted := env.do(t, http.MethodPost, "/ui/torrserver/command", url.Values{"cmd": {"action:delete"}}, csrf, true)
	if deleted.status != http.StatusOK {
		t.Fatalf("delete = %d, body=%s", deleted.status, deleted.body)
	}
	if len(env.ts.removed) != 1 || env.ts.removed[0] != "h1" {
		t.Errorf("removed = %v", env.ts.removed)
	}
	if strings.Contains(deleted.body, `<span class="title">First</span>`) {
		t.Errorf("deleted Torrserver item is still rendered: %s", deleted.body)
	}
	if !strings.Contains(deleted.body, "Second") {
		t.Errorf("the remaining Torrserver item disappeared: %s", deleted.body)
	}
}

func TestStaleSelectionReturnsConflict(t *testing.T) {
	env := newTestEnv(t, "/")
	env.jackett.results = []jackett.Result{{Title: "one", InfoHash: "h1"}}
	csrf := env.csrf(t)
	env.do(t, http.MethodPost, "/find/search", url.Values{"q": {"x"}}, csrf, true)
	res := env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"select:99"}}, csrf, true)
	if res.status != http.StatusConflict {
		t.Errorf("stale select = %d, want 409", res.status)
	}
	if !strings.Contains(res.body, "list changed") && !strings.Contains(res.body, "The list changed") {
		t.Errorf("stale response lacks a message: %s", res.body)
	}
}

func TestExpiredSessionHtmxRedirect(t *testing.T) {
	env := newTestEnv(t, "/")
	env.login(t)
	env.server.store.now = func() time.Time { return time.Now().Add(200 * time.Hour) }
	res := env.do(t, http.MethodGet, "/find", nil, "", true)
	if res.status != http.StatusOK {
		t.Errorf("expired htmx session = %d, want 200", res.status)
	}
	if got := res.header.Get("HX-Redirect"); !strings.Contains(got, "/login") {
		t.Errorf("HX-Redirect = %q", got)
	}
}

func TestLogoutDoesNotAffectAnotherSession(t *testing.T) {
	env := newTestEnv(t, "/")
	first := env.login(t)
	other := *env
	jar, _ := cookiejar.New(nil)
	other.client = &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	second := other.login(t)
	if first.User != second.User {
		t.Fatal("unexpected users")
	}
	res := env.do(t, http.MethodPost, "/logout", url.Values{"csrf": {first.CSRF}}, "", false)
	if res.status != http.StatusSeeOther {
		t.Fatalf("logout = %d", res.status)
	}
	page := other.do(t, http.MethodGet, "/find", nil, "", false)
	if page.status != http.StatusOK {
		t.Errorf("second session was logged out too: %d", page.status)
	}
}

func TestBasePathFallback(t *testing.T) {
	env := newTestEnv(t, "/torrentino")

	res := env.do(t, http.MethodGet, "/find", nil, "", false)
	if res.status != http.StatusSeeOther {
		t.Fatalf("GET /torrentino/find = %d", res.status)
	}
	if got, want := res.header.Get("Location"), "/torrentino/login?next=%2Ftorrentino%2Ffind"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if res := env.doPath(t, http.MethodGet, "/find", nil, "", false); res.status != http.StatusNotFound {
		t.Errorf("GET /find outside prefix = %d, want 404", res.status)
	}

	csrf := env.csrf(t)
	if got := env.cookie(sessionCookie); got == "" {
		t.Fatal("session cookie is missing")
	}
	// The cookie Path must include the prefix. The jar does not expose Path,
	// so inspect the login response headers via a fresh client.
	jar, _ := cookiejar.New(nil)
	fresh := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	loginPage, _ := http.NewRequest(http.MethodGet, env.url("/login"), nil)
	loginRes, err := fresh.Do(loginPage)
	if err != nil {
		t.Fatal(err)
	}
	loginRes.Body.Close()
	lookup := *env.baseURL
	lookup.Path = env.prefix() + "/"
	loginToken := ""
	for _, c := range jar.Cookies(&lookup) {
		if c.Name == loginCookie {
			loginToken = c.Value
		}
	}
	if loginToken == "" {
		t.Fatal("pre-auth cookie is missing for the prefixed login")
	}
	loginForm, _ := http.NewRequest(http.MethodPost, env.url("/login"),
		strings.NewReader(url.Values{"username": {"denis"}, "password": {"secret"}, "csrf": {loginToken}}.Encode()))
	loginForm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResp, err := fresh.Do(loginForm)
	if err != nil {
		t.Fatal(err)
	}
	loginResp.Body.Close()
	foundSessionCookie := false
	for _, header := range loginResp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(header, sessionCookie+"=") {
			foundSessionCookie = true
			if !strings.Contains(header, "Path=/torrentino") {
				t.Errorf("session cookie does not use the prefix path: %q", header)
			}
		}
	}
	if !foundSessionCookie {
		t.Error("prefixed login did not set a session cookie")
	}

	root := env.do(t, http.MethodGet, "/", nil, "", false)
	if root.status != http.StatusSeeOther || root.header.Get("Location") != "/torrentino/find" {
		t.Errorf("GET /torrentino/ = %d, Location=%q", root.status, root.header.Get("Location"))
	}
	if page := env.do(t, http.MethodGet, "/find", nil, "", false); page.status != http.StatusOK {
		t.Errorf("GET /torrentino/find after login = %d", page.status)
	}
	if static := env.do(t, http.MethodGet, "/static/app.css", nil, "", false); static.status != http.StatusOK {
		t.Errorf("prefixed static = %d", static.status)
	}
	_ = csrf
}

func TestUnknownPathIs404(t *testing.T) {
	env := newTestEnv(t, "/")
	env.login(t)
	if res := env.do(t, http.MethodGet, "/nope", nil, "", false); res.status != http.StatusNotFound {
		t.Errorf("GET /nope = %d, want 404", res.status)
	}
}

func TestSecurityHeadersAndNoStore(t *testing.T) {
	env := newTestEnv(t, "/")
	res := env.do(t, http.MethodGet, "/healthz", nil, "", false)
	for _, header := range []string{"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy", "X-Frame-Options"} {
		if res.header.Get(header) == "" {
			t.Errorf("%s header is missing", header)
		}
	}
	csp := res.header.Get("Content-Security-Policy")
	if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP contains unsafe directives: %q", csp)
	}
	if res.header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", res.header.Get("Cache-Control"))
	}
}

func TestSecureCookies(t *testing.T) {
	env := newTestEnv(t, "/")
	common.Settings.Web.SecureCookies = true
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, _ := http.NewRequest(http.MethodGet, env.url("/login"), nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	found := false
	for _, header := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(header, loginCookie+"=") {
			found = true
			if !strings.Contains(header, "Secure") {
				t.Errorf("pre-auth cookie is not Secure: %q", header)
			}
		}
	}
	if !found {
		t.Error("pre-auth cookie is missing")
	}
}

func TestNonHtmxPRG(t *testing.T) {
	env := newTestEnv(t, "/")
	csrf := env.csrf(t)
	env.jackett.results = []jackett.Result{{Title: "result", InfoHash: "h1"}}
	res := env.do(t, http.MethodPost, "/find/search", url.Values{"q": {"x"}}, csrf, false)
	if res.status != http.StatusSeeOther || res.header.Get("Location") != "/find" {
		t.Fatalf("non-htmx search = %d, Location=%q", res.status, res.header.Get("Location"))
	}
	page := env.do(t, http.MethodGet, "/find", nil, "", false)
	if !strings.Contains(page.body, "result") {
		t.Errorf("search result is missing after PRG: %s", page.body)
	}
	command := env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"select:0"}}, csrf, false)
	if command.status != http.StatusSeeOther || command.header.Get("Location") != "/find" {
		t.Errorf("non-htmx command = %d, Location=%q", command.status, command.header.Get("Location"))
	}
}

func TestFragmentQueryWithoutHtmx(t *testing.T) {
	env := newTestEnv(t, "/")
	env.login(t)
	res := env.do(t, http.MethodGet, "/downloads?fragment=1", nil, "", false)
	if res.status != http.StatusOK {
		t.Fatalf("fragment = %d", res.status)
	}
	if strings.Contains(res.body, "<!doctype html>") {
		t.Error("fragment query returned a full page")
	}
	if !strings.Contains(res.body, `id="screen"`) {
		t.Error("fragment does not contain the screen root")
	}
}

func TestTorrentFromHTMLIs502(t *testing.T) {
	env := newTestEnv(t, "/")
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>not a torrent</html>"))
	}))
	defer page.Close()
	env.jackett.results = []jackett.Result{{
		Title: "bad", InfoHash: "h1", Link: page.URL + "/x.torrent",
	}}
	csrf := env.csrf(t)
	env.do(t, http.MethodPost, "/find/search", url.Values{"q": {"x"}}, csrf, true)
	res := env.do(t, http.MethodGet, "/find/torrent?item=0", nil, "", false)
	if res.status != http.StatusBadGateway {
		t.Errorf("HTML torrent = %d, want 502", res.status)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	env := newTestEnv(t, "/")
	env.login(t)
	if res := env.do(t, http.MethodPost, "/find", url.Values{}, "", false); res.status != http.StatusMethodNotAllowed {
		t.Errorf("POST /find = %d, want 405", res.status)
	}
}

func TestSearchRateLimit(t *testing.T) {
	env := newTestEnv(t, "/")
	env.jackett.results = []jackett.Result{{Title: "one", InfoHash: "h1"}}
	csrf := env.csrf(t)
	for i := 0; i < 10; i++ {
		res := env.do(t, http.MethodPost, "/find/search", url.Values{"q": {"x"}}, csrf, true)
		if res.status != http.StatusOK {
			t.Fatalf("search %d = %d", i, res.status)
		}
	}
	res := env.do(t, http.MethodPost, "/find/search", url.Values{"q": {"x"}}, csrf, true)
	if res.status != http.StatusTooManyRequests {
		t.Fatalf("search 11 = %d, want 429", res.status)
	}
	if res.header.Get("Retry-After") == "" {
		t.Error("Retry-After header is missing")
	}
	if !strings.Contains(res.body, "Too many searches") {
		t.Errorf("rate-limit banner is missing: %s", res.body)
	}
}

func TestMultipleAccounts(t *testing.T) {
	env := newTestEnv(t, "/")
	hash, err := password.Hash("family-secret", password.MinIterations)
	if err != nil {
		t.Fatal(err)
	}
	common.Settings.Web.Users = append(common.Settings.Web.Users, common.WebUser{Name: "family", PasswordHash: hash})
	env.server.users["family"] = common.WebUser{Name: "family", PasswordHash: hash}

	first := env.login(t)
	secondEnv := *env
	jar, _ := cookiejar.New(nil)
	secondEnv.client = &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	secondEnv.loginName = "family"
	secondEnv.password = "family-secret"
	second := secondEnv.login(t)
	if first == second {
		t.Fatal("both accounts got the same session")
	}
	if first.User != "denis" || second.User != "family" {
		t.Errorf("users = %q, %q", first.User, second.User)
	}
	if res := secondEnv.do(t, http.MethodGet, "/find", nil, "", false); res.status != http.StatusOK {
		t.Errorf("second account /find = %d", res.status)
	}
}

func TestLoginNextIsSanitized(t *testing.T) {
	env := newTestEnv(t, "/")
	env.do(t, http.MethodGet, "/login?next=//evil.example", nil, "", false)
	token := env.cookie(loginCookie)
	res := env.do(t, http.MethodPost, "/login", url.Values{
		"username": {"denis"}, "password": {"secret"}, "csrf": {token},
		"next": {"//evil.example"},
	}, "", false)
	if res.status != http.StatusSeeOther {
		t.Fatalf("login = %d", res.status)
	}
	if got := res.header.Get("Location"); got != "/find" {
		t.Errorf("Location = %q, want /find", got)
	}
}

func TestDownloadsUpdateFailureKeepsBanner(t *testing.T) {
	env := newTestEnv(t, "/")
	env.tr.listErr = io.ErrUnexpectedEOF
	env.login(t)
	res := env.do(t, http.MethodGet, "/downloads", nil, "", false)
	if res.status != http.StatusOK {
		t.Fatalf("downloads = %d", res.status)
	}
	if !strings.Contains(res.body, "Update failed") || !strings.Contains(res.body, "Reload") {
		t.Errorf("update failure banner is missing: %s", res.body)
	}
}

func TestSelectSameRowDeselects(t *testing.T) {
	env := newTestEnv(t, "/")
	env.jackett.results = []jackett.Result{{Title: "one", InfoHash: "h1"}}
	csrf := env.csrf(t)
	env.do(t, http.MethodPost, "/find/search", url.Values{"q": {"x"}}, csrf, true)
	first := env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"select:0"}}, csrf, true)
	if !strings.Contains(first.body, `class="row selected"`) {
		t.Fatalf("first select did not select: %s", first.body)
	}
	second := env.do(t, http.MethodPost, "/ui/find/command", url.Values{"cmd": {"select:0"}}, csrf, true)
	if strings.Contains(second.body, `class="row selected"`) {
		t.Errorf("second select did not deselect: %s", second.body)
	}
}
