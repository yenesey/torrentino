package core

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hekmon/transmissionrpc/v2"

	"torrentino/api/jackett"
	"torrentino/api/torrserver"
	"torrentino/common"
)

type fakeJackett struct {
	results  []jackett.Result
	queryErr error
	hash     string
	hashErr  error
	queries  []string
}

func (f *fakeJackett) Query(ctx context.Context, query string, indexers []string) ([]jackett.Result, error) {
	f.queries = append(f.queries, query)
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
	started  []int64
	paused   []int64
	addErr   error
}

func (f *fakeTransmission) List(ctx context.Context) ([]transmissionrpc.Torrent, error) {
	return f.torrents, f.listErr
}
func (f *fakeTransmission) Add(ctx context.Context, link string, dir string) error {
	f.added = append(f.added, link+"|"+dir)
	return f.addErr
}
func (f *fakeTransmission) Delete(ctx context.Context, id int64) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeTransmission) Start(ctx context.Context, id int64) error {
	f.started = append(f.started, id)
	return nil
}
func (f *fakeTransmission) Pause(ctx context.Context, id int64) error {
	f.paused = append(f.paused, id)
	return nil
}

type fakeFlareSolverr struct {
	html string
	err  error
}

func (f *fakeFlareSolverr) Fetch(ctx context.Context, target string) (string, error) {
	return f.html, f.err
}

type fakeTorrserver struct {
	items   []torrserver.TSListItem
	adds    []string
	removed []string
	listErr error
	addErr  error
}

func (f *fakeTorrserver) List(ctx context.Context) ([]torrserver.TSListItem, error) {
	return f.items, f.listErr
}
func (f *fakeTorrserver) Add(ctx context.Context, link string, title string, poster string) error {
	f.adds = append(f.adds, link+"|"+title+"|"+poster)
	return f.addErr
}
func (f *fakeTorrserver) Delete(ctx context.Context, hash string) error {
	f.removed = append(f.removed, hash)
	return nil
}

func testService() (*Service, *fakeJackett, *fakeTransmission, *fakeTorrserver) {
	j := &fakeJackett{}
	t := &fakeTransmission{}
	r := &fakeTorrserver{}
	return &Service{Jackett: j, Transmission: t, Torrserver: r}, j, t, r
}

func ptr[T any](value T) *T { return &value }

func TestSearchMarksExistingItems(t *testing.T) {
	service, j, tr, ts := testService()
	j.results = []jackett.Result{
		{Title: "in both", InfoHash: "AA"},
		{Title: "only torrent", InfoHash: "bb"},
		{Title: "fresh", InfoHash: "cc"},
	}
	tr.torrents = []transmissionrpc.Torrent{{HashString: ptr("aa")}}
	ts.items = []torrserver.TSListItem{{Hash: "aa"}, {Hash: "bb"}}

	items, err := service.Search(context.Background(), "query")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items", len(items))
	}
	if !items[0].InTorrents || !items[0].InTorrserver {
		t.Errorf("item 0 flags = %+v", items[0])
	}
	if items[1].InTorrents || !items[1].InTorrserver {
		t.Errorf("item 1 flags = %+v", items[1])
	}
	if items[2].InTorrents || items[2].InTorrserver {
		t.Errorf("item 2 flags = %+v", items[2])
	}
}

func TestSearchUpstreamError(t *testing.T) {
	service, j, _, _ := testService()
	j.queryErr = errors.New("jackett down")
	_, err := service.Search(context.Background(), "q")
	if !errors.Is(err, ErrUpstream) {
		t.Errorf("error = %v, want ErrUpstream", err)
	}
}

func TestSearchValidatesQuery(t *testing.T) {
	service, _, _, _ := testService()
	if _, err := service.Search(context.Background(), "   "); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty query error = %v", err)
	}
	if _, err := service.Search(context.Background(), strings.Repeat("x", 257)); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("long query error = %v", err)
	}
}

func TestResolveHash(t *testing.T) {
	service, j, _, _ := testService()
	j.hash = "resolved"
	item := &FindItem{Result: jackett.Result{Link: "http://tracker/a.torrent"}}
	if err := service.ResolveHash(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if item.InfoHash != "resolved" {
		t.Errorf("InfoHash = %q", item.InfoHash)
	}
	// An existing hash is not looked up again.
	j.hash = "other"
	if err := service.ResolveHash(context.Background(), item); err != nil || item.InfoHash != "resolved" {
		t.Errorf("ResolveHash changed an existing hash: %q, %v", item.InfoHash, err)
	}
}

func TestFindActions(t *testing.T) {
	service, _, _, _ := testService()
	item := &FindItem{Result: jackett.Result{Link: "http://x/a", Details: "http://x/details"}, TorrentAvailable: true}
	ids := ActionIDs(service.FindActions(item))
	if strings.Join(ids, ",") != "->default,->series,->movie,torrsrv,.torrent,web page" {
		t.Errorf("actions = %v", ids)
	}

	item.InTorrents = true
	item.InTorrserver = true
	item.TorrentAvailable = false
	item.Result.Link = ""
	item.Result.MagnetUri = "magnet:?xt=urn:btih:x"
	item.Result.Details = "javascript:alert(1)"
	ids = ActionIDs(service.FindActions(item))
	if strings.Join(ids, ",") != "" {
		t.Errorf("actions for an existing magnet result = %v", ids)
	}

	unavailable := &FindItem{Result: jackett.Result{Link: "http://x/a"}}
	ids = ActionIDs(service.FindActions(unavailable))
	if strings.Join(ids, ",") != "->default,->series,->movie,torrsrv" {
		t.Errorf("actions for an unavailable .torrent = %v", ids)
	}
}

func TestDoFindDownloadsToConfiguredDirectory(t *testing.T) {
	old := common.Settings.Path
	t.Cleanup(func() { common.Settings.Path = old })
	common.Settings.Path.Default = "/default"
	common.Settings.Path.Movie = "/movie"
	common.Settings.Path.Series = "/series"

	for _, c := range []struct {
		action string
		want   string
	}{
		{"->default", "/default"},
		{"->movie", "/movie"},
		{"->series", "/series"},
	} {
		service, _, tr, _ := testService()
		item := &FindItem{Result: jackett.Result{Link: "http://x/a"}}
		if err := service.DoFind(context.Background(), item, c.action); err != nil {
			t.Fatalf("%s: %v", c.action, err)
		}
		if len(tr.added) != 1 || tr.added[0] != "http://x/a|"+c.want {
			t.Errorf("%s: added = %v", c.action, tr.added)
		}
		if !item.InTorrents {
			t.Errorf("%s: InTorrents = false", c.action)
		}
	}
}

func TestDoFindTorrsrv(t *testing.T) {
	service, _, _, ts := testService()
	item := &FindItem{Result: jackett.Result{MagnetUri: "magnet:?xt=urn:btih:x", Title: "title"}}
	if err := service.DoFind(context.Background(), item, "torrsrv"); err != nil {
		t.Fatal(err)
	}
	if len(ts.adds) != 1 || ts.adds[0] != "magnet:?xt=urn:btih:x|title|" {
		t.Errorf("adds = %v", ts.adds)
	}
	if !item.InTorrserver {
		t.Error("InTorrserver = false")
	}
}

func TestDoFindRejectsUnknownAndLinkActions(t *testing.T) {
	service, _, _, _ := testService()
	item := &FindItem{Result: jackett.Result{Link: "http://x/a"}}
	for _, action := range []string{"unknown", ".torrent", "web page"} {
		if err := service.DoFind(context.Background(), item, action); !errors.Is(err, ErrNotAvailable) {
			t.Errorf("%s: error = %v, want ErrNotAvailable", action, err)
		}
	}
}

func TestTorrentFileChecksContentAndSize(t *testing.T) {
	valid := []byte("d4:infod4:name4:teste e")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good":
			_, _ = w.Write(valid)
		case "/html":
			_, _ = w.Write([]byte("<html>nope</html>"))
		case "/big":
			w.Header().Set("Content-Length", "30000000")
			_, _ = w.Write(make([]byte, 26<<20))
		}
	}))
	defer server.Close()

	service, _, _, _ := testService()
	data, err := service.TorrentFile(context.Background(), server.URL+"/good")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(valid) {
		t.Errorf("data = %q", data)
	}
	if _, err := service.TorrentFile(context.Background(), server.URL+"/html"); !errors.Is(err, ErrUpstream) {
		t.Errorf("html error = %v, want ErrUpstream", err)
	}
	if _, err := service.TorrentFile(context.Background(), server.URL+"/big"); !errors.Is(err, ErrTorrentTooLarge) {
		t.Errorf("big error = %v, want ErrTorrentTooLarge", err)
	}
	if _, err := service.TorrentFile(context.Background(), "file:///etc/passwd"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("file URL error = %v, want ErrInvalidInput", err)
	}
}

func TestDownloadsIncludesDiskOnlyAndStats(t *testing.T) {
	root := t.TempDir()
	old := common.Settings.Path
	t.Cleanup(func() { common.Settings.Path = old })
	common.Settings.Path.Default = root
	common.Settings.Path.Movie = ""
	common.Settings.Path.Series = ""

	if err := os.Mkdir(filepath.Join(root, "OnDisk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "OnDisk", "a.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	service, _, tr, _ := testService()
	tr.torrents = []transmissionrpc.Torrent{{
		ID:             ptr[int64](1),
		Name:           ptr("InTransmission"),
		HashString:     ptr("aa"),
		DownloadDir:    ptr(root),
		DownloadedEver: ptr[int64](100),
		PercentDone:    ptr(1.0),
		UploadRatio:    ptr(2.0),
		AddedDate:      ptr(time.Now()),
		Status:         ptr(transmissionrpc.TorrentStatusSeed),
		Files:          []*transmissionrpc.TorrentFile{{Name: "movie.mkv"}},
	}}

	items, stats, err := service.Downloads(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if !stats.OK || stats.Total == 0 {
		t.Errorf("stats = %+v", stats)
	}
	var disk *DownloadItem
	for _, item := range items {
		if item.Status == "unknown" {
			disk = item
		}
	}
	if disk == nil {
		t.Fatal("disk-only item is missing")
	}
	if disk.NameValue() != "OnDisk" || !disk.IsDir {
		t.Errorf("disk item = %+v", disk)
	}
	if disk.Ext != ".mkv" || disk.ExtCount != 1 {
		t.Errorf("disk ext = %q count = %d", disk.Ext, disk.ExtCount)
	}
}

func TestDownloadPathRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	old := common.Settings.Path
	t.Cleanup(func() { common.Settings.Path = old })
	common.Settings.Path.Default = root

	service, _, _, _ := testService()
	good := &DownloadItem{Torrent: transmissionrpc.Torrent{
		Name:        ptr("file.mkv"),
		DownloadDir: ptr(root),
	}}
	path, err := service.DownloadPath(good)
	if err != nil || path != filepath.Join(root, "file.mkv") {
		t.Errorf("good path = %q, %v", path, err)
	}

	bad := &DownloadItem{Torrent: transmissionrpc.Torrent{
		Name:        ptr("../escape"),
		DownloadDir: ptr(root),
	}}
	if _, err := service.DownloadPath(bad); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("traversal error = %v, want ErrUnsafePath", err)
	}
}

func TestDoDownload(t *testing.T) {
	root := t.TempDir()
	old := common.Settings.Path
	t.Cleanup(func() { common.Settings.Path = old })
	common.Settings.Path.Default = root

	service, _, tr, _ := testService()
	known := &DownloadItem{Torrent: transmissionrpc.Torrent{
		ID:     ptr[int64](7),
		Name:   ptr("known"),
		Status: ptr(transmissionrpc.TorrentStatusSeed),
	}}
	if err := service.DoDownload(context.Background(), known, "pause"); err != nil {
		t.Fatal(err)
	}
	if err := service.DoDownload(context.Background(), known, "delete"); err != nil {
		t.Fatal(err)
	}
	if len(tr.paused) != 1 || tr.paused[0] != 7 || len(tr.deleted) != 1 || tr.deleted[0] != 7 {
		t.Errorf("paused=%v deleted=%v", tr.paused, tr.deleted)
	}

	target := filepath.Join(root, "diskfile")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	disk := &DownloadItem{Torrent: transmissionrpc.Torrent{
		Name:        ptr("diskfile"),
		DownloadDir: ptr(root),
		Status:      ptr(transmissionrpc.TorrentStatus(404)),
	}, Status: "unknown"}
	if err := service.DoDownload(context.Background(), disk, "delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("file still exists: %v", err)
	}
}

func TestDownloadActions(t *testing.T) {
	service, _, _, _ := testService()
	for status, want := range map[string]string{
		"downloading": "pause,delete",
		"seeding":     "pause,delete",
		"paused":      "start,delete",
		"idle":        "start,delete",
		"unknown":     "delete",
	} {
		ids := strings.Join(ActionIDs(service.DownloadActions(&DownloadItem{Status: status})), ",")
		if ids != want {
			t.Errorf("%s: actions = %q, want %q", status, ids, want)
		}
	}
}

func TestDownloadKey(t *testing.T) {
	if got := DownloadKey(&DownloadItem{Torrent: transmissionrpc.Torrent{ID: ptr[int64](3)}}); got != "id:3" {
		t.Errorf("key = %q", got)
	}
	if got := DownloadKey(&DownloadItem{Torrent: transmissionrpc.Torrent{
		Name: ptr("a"), DownloadDir: ptr("/x"),
	}}); got != "path:/x/a" {
		t.Errorf("key = %q", got)
	}
}

func TestTorrserver(t *testing.T) {
	service, _, _, ts := testService()
	ts.items = []torrserver.TSListItem{{Title: "a", Torrent_Size: 10, Hash: "h1"}}
	items, err := service.TorrserverList(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	if ids := ActionIDs(service.TorrserverActions(items[0])); len(ids) != 1 || ids[0] != "delete" {
		t.Errorf("actions = %v", ids)
	}
	if err := service.DoTorrserver(context.Background(), items[0], "delete"); err != nil {
		t.Fatal(err)
	}
	if len(ts.removed) != 1 || ts.removed[0] != "h1" {
		t.Errorf("removed = %v", ts.removed)
	}
	if TorrserverKey(items[0]) != "hash:h1" {
		t.Errorf("key = %q", TorrserverKey(items[0]))
	}
}

func TestPosterPrefersJackettResult(t *testing.T) {
	service, _, _, _ := testService()
	item := &FindItem{Result: jackett.Result{
		Poster:  "http://tracker/poster.jpg",
		Details: "http://127.0.0.1:1/unreachable",
	}}
	if got := service.Poster(context.Background(), item); got != "http://tracker/poster.jpg" {
		t.Errorf("poster = %q", got)
	}
}

func TestPosterFallsBackToImgHeuristic(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.UserAgent(), "Mozilla/5.0") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`<html><body>
			<img class="logo" src="/logo.png" width="500" height="500">
			<img class="poster" alt="Poster" src="/poster.jpg">
		</body></html>`))
	}))
	defer page.Close()
	service, _, _, _ := testService()
	item := &FindItem{Result: jackett.Result{Details: page.URL}}
	if got, want := service.Poster(context.Background(), item), page.URL+"/poster.jpg"; got != want {
		t.Errorf("poster = %q, want %q", got, want)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<img class="poster" src="javascript:alert(1)">`))
	}))
	defer bad.Close()
	item = &FindItem{Result: jackett.Result{Details: bad.URL}}
	if got := service.Poster(context.Background(), item); got != "" {
		t.Errorf("javascript poster = %q, want empty", got)
	}
}

func TestProbeTorrent(t *testing.T) {
	service, j, _, _ := testService()
	j.hash = "abc"
	item := &FindItem{Result: jackett.Result{Link: "http://x/a"}}
	if err := service.ProbeTorrent(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if !item.TorrentAvailable || item.InfoHash != "abc" {
		t.Errorf("available=%v hash=%q", item.TorrentAvailable, item.InfoHash)
	}

	j.hashErr = errors.New("404")
	item = &FindItem{Result: jackett.Result{Link: "http://x/a"}}
	if err := service.ProbeTorrent(context.Background(), item); err == nil {
		t.Fatal("ProbeTorrent succeeded for a broken link")
	}
	if item.TorrentAvailable {
		t.Error("TorrentAvailable is true after a failed probe")
	}
}

func TestDoFindMapsTorrserverBadRequest(t *testing.T) {
	service, _, _, ts := testService()
	ts.addErr = torrserver.ErrBadRequest
	item := &FindItem{Result: jackett.Result{Link: "http://x/a"}}
	err := service.DoFind(context.Background(), item, "torrsrv")
	if !errors.Is(err, ErrTorrentUnavailable) {
		t.Errorf("error = %v, want ErrTorrentUnavailable", err)
	}
}

func TestPosterFallsBackToFlareSolverr(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer page.Close()
	service, _, _, _ := testService()
	service.FlareSolverr = &fakeFlareSolverr{
		html: `<html><body>
			<img class="logo" alt="logo" src="/logo.png">
			<img class="p200" src="/big/2026/1018/cover.jpg">
		</body></html>`,
	}
	item := &FindItem{Result: jackett.Result{Details: page.URL}}
	if got, want := service.Poster(context.Background(), item), page.URL+"/big/2026/1018/cover.jpg"; got != want {
		t.Errorf("poster = %q, want %q", got, want)
	}
}
