package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"

	"torrentino/api/jackett"
	"torrentino/common"
	"torrentino/common/utils"
)

const (
	serviceTimeout  = 10 * time.Second
	findTimeout     = 15 * time.Second
	torrentTimeout  = 30 * time.Second
	torrentMaxBytes = 25 << 20
	posterTimeout   = 10 * time.Second
	posterMaxBytes  = 2 << 20
)

// FindItem is one Jackett search result plus the two "already exists" flags.
type FindItem struct {
	jackett.Result
	InTorrents       bool
	InTorrserver     bool
	TorrentAvailable bool
}

// Search queries Jackett and marks results that already exist in Transmission
// or Torrserver. A failure to read either local service is logged but does not
// fail the search: the user still gets the Jackett results (without markers).
func (s *Service) Search(ctx context.Context, query string) ([]*FindItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: query is empty", ErrInvalidInput)
	}
	if len([]rune(query)) > 256 {
		return nil, fmt.Errorf("%w: query is longer than 256 characters", ErrInvalidInput)
	}

	results, err := s.Jackett.Query(ctx, query, common.Settings.Jackett.Indexers)
	if err != nil {
		return nil, Upstream(err)
	}

	transmissionHashes := make(map[string]bool)
	listCtx, cancel := context.WithTimeout(ctx, serviceTimeout)
	list, listErr := s.Transmission.List(listCtx)
	cancel()
	if listErr != nil {
		utils.LogError(listErr)
	} else {
		for i := range list {
			if list[i].HashString != nil {
				transmissionHashes[strings.ToLower(*list[i].HashString)] = true
			}
		}
	}

	torrserverHashes := make(map[string]bool)
	listCtx, cancel = context.WithTimeout(ctx, serviceTimeout)
	tsList, tsErr := s.Torrserver.List(listCtx)
	cancel()
	if tsErr != nil {
		utils.LogError(tsErr)
	} else {
		for i := range tsList {
			torrserverHashes[strings.ToLower(tsList[i].Hash)] = true
		}
	}

	items := make([]*FindItem, 0, len(results))
	for i := range results {
		hash := strings.ToLower(results[i].InfoHash)
		items = append(items, &FindItem{
			Result:       results[i],
			InTorrents:   hash != "" && transmissionHashes[hash],
			InTorrserver: hash != "" && torrserverHashes[hash],
		})
	}
	return items, nil
}

// ProbeTorrent checks whether the result's .torrent can actually be fetched
// and parsed. It also fills InfoHash when the result did not provide one.
func (s *Service) ProbeTorrent(ctx context.Context, item *FindItem) error {
	if item == nil {
		return ErrStaleItem
	}
	if !safeHTTPURL(item.Link) {
		return fmt.Errorf("%w: result has no valid .torrent link", ErrNotAvailable)
	}
	hash, err := s.Jackett.GetInfoHash(ctx, item.Link)
	if err != nil {
		item.TorrentAvailable = false
		return Upstream(err)
	}
	if hash != "" {
		item.InfoHash = hash
	}
	item.TorrentAvailable = true
	return nil
}

// ResolveHash fills InfoHash for results that only expose a .torrent link.
// The lookup has its own 5-second deadline inside the Jackett client.
func (s *Service) ResolveHash(ctx context.Context, item *FindItem) error {
	if item == nil || item.InfoHash != "" || item.Link == "" {
		return nil
	}
	hash, err := s.Jackett.GetInfoHash(ctx, item.Link)
	if err != nil {
		return Upstream(err)
	}
	item.InfoHash = hash
	return nil
}

// FindCompare implements the search sorting rules.
func FindCompare(a, b *FindItem, attribute string) bool {
	switch attribute {
	case "Size":
		return a.Size < b.Size
	case "Seeders":
		return a.Seeders < b.Seeders
	case "Peers":
		return a.Peers < b.Peers
	case "Link":
		if a.Link == "" && b.Link != "" {
			return true
		}
		return false
	}
	return false
}

// FindActions returns the actions offered for a search result, in the same
// order the Telegram keyboard uses.
func (s *Service) FindActions(item *FindItem) []Action {
	var result []Action
	if !item.InTorrents {
		result = append(result,
			Action{ID: "->default", Label: "->default"},
			Action{ID: "->series", Label: "->series"},
			Action{ID: "->movie", Label: "->movie"},
		)
	}
	if !item.InTorrserver && (item.MagnetUri != "" || (item.TorrentAvailable && item.Link != "")) {
		result = append(result, Action{ID: "torrsrv", Label: "torrsrv"})
	}
	if item.TorrentAvailable && item.Link != "" {
		result = append(result, Action{ID: ".torrent", Label: ".torrent"})
	}
	if safeHTTPURL(item.Details) {
		result = append(result, Action{ID: "web page", Label: "web page"})
	}
	return result
}

// FindKey is the stable identity of a search result across reloads.
func FindKey(item *FindItem) string {
	switch {
	case item == nil:
		return ""
	case item.InfoHash != "":
		return "hash:" + strings.ToLower(item.InfoHash)
	case item.Link != "":
		return "link:" + item.Link
	default:
		return "title:" + item.Title
	}
}

// DoFind executes a non-link action for a search result. ".torrent" and
// "web page" are transport concerns handled by each UI (document upload or a
// browser download/link), so they are not domain actions.
func (s *Service) DoFind(ctx context.Context, item *FindItem, action string) error {
	if item == nil {
		return ErrStaleItem
	}
	ctx, cancel := context.WithTimeout(ctx, findTimeout)
	defer cancel()
	link := item.Link
	if link == "" {
		link = item.MagnetUri
	}

	switch action {
	case "->default", "->series", "->movie":
		if link == "" {
			return fmt.Errorf("%w: result has neither a link nor a magnet", ErrNotAvailable)
		}
		dir := common.Settings.Path.Default
		switch action {
		case "->series":
			dir = common.Settings.Path.Series
		case "->movie":
			dir = common.Settings.Path.Movie
		}
		if err := s.Transmission.Add(ctx, link, dir); err != nil {
			return Upstream(err)
		}
		item.InTorrents = true
		return nil
	case "torrsrv":
		// Prefer a magnet: Torrserver fetches it itself. A .torrent link is
		// only usable after the availability probe succeeded; otherwise the
		// upstream returns a bare 400 and the UI misreports it as an outage.
		torrLink := item.MagnetUri
		if torrLink == "" {
			if !item.TorrentAvailable || item.Link == "" {
				return fmt.Errorf("%w: .torrent is unavailable", ErrNotAvailable)
			}
			torrLink = item.Link
		}
		poster := s.Poster(ctx, item)
		if err := s.Torrserver.Add(ctx, torrLink, item.Title, poster); err != nil {
			return Upstream(err)
		}
		item.InTorrserver = true
		return nil
	case ".torrent", "web page":
		return fmt.Errorf("%w: %s is handled by the UI", ErrNotAvailable, action)
	default:
		return fmt.Errorf("%w: %q", ErrNotAvailable, action)
	}
}

// TorrentFile downloads a .torrent from a search result. The URL always comes
// from the server-side item, never from the request. Only http(s) is accepted,
// the body is limited to 25 MiB and must start with 'd' (a bencode dictionary).
func (s *Service) TorrentFile(ctx context.Context, link string) ([]byte, error) {
	if !safeHTTPURL(link) {
		return nil, fmt.Errorf("%w: only http(s) torrent URLs are allowed", ErrInvalidInput)
	}

	ctx, cancel := context.WithTimeout(ctx, torrentTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 3 {
				return fmt.Errorf("too many redirects")
			}
			if !safeHTTPURL(req.URL.String()) {
				return fmt.Errorf("redirect to a non-http(s) URL")
			}
			return nil
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, Upstream(fmt.Errorf("request failed: %v", requestErrorDetail(err)))
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, Upstream(fmt.Errorf("tracker returned %s", res.Status))
	}

	data, err := io.ReadAll(io.LimitReader(res.Body, torrentMaxBytes+1))
	if err != nil {
		return nil, Upstream(err)
	}
	if len(data) > torrentMaxBytes {
		return nil, ErrTorrentTooLarge
	}
	if len(data) == 0 || data[0] != 'd' {
		return nil, Upstream(fmt.Errorf("response is not a bencode dictionary"))
	}
	return data, nil
}

// Poster returns the Torrserver poster for a search result. It prefers the
// Poster field supplied by Jackett and falls back to a tracker-page <img>
// heuristic; there are no tracker-specific XPath rules to keep in sync.
func (s *Service) Poster(ctx context.Context, item *FindItem) string {
	if item == nil {
		return ""
	}
	if poster := resolvePosterURL(item.Details, item.Poster); poster != "" {
		return poster
	}
	if !safeHTTPURL(item.Details) {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, posterTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.Details, nil)
	if err != nil {
		return ""
	}
	// Many trackers reject empty/Go user agents with 403 even when the page is
	// otherwise public. Use a normal browser fingerprint for this HTML fetch.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		utils.LogError(fmt.Errorf("poster request failed: %v", requestErrorDetail(err)))
		return ""
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		utils.LogError(fmt.Errorf("poster page returned %s", res.Status))
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, posterMaxBytes))
	if err != nil {
		utils.LogError(err)
		return ""
	}

	doc, err := htmlquery.Parse(bytes.NewReader(data))
	if err != nil {
		utils.LogError(err)
		return ""
	}
	return posterFromDoc(doc, item.Details)
}

func posterFromDoc(doc *html.Node, pageURL string) string {
	best := ""
	bestScore := 0
	for _, img := range htmlquery.Find(doc, "//img") {
		raw := firstImageAttr(img, "src", "data-src", "data-original", "data-lazy-src", "data-url")
		src := resolvePosterURL(pageURL, raw)
		if src == "" {
			continue
		}
		score := posterScore(img, src)
		if score > bestScore {
			best = src
			bestScore = score
		}
	}
	return best
}

func posterScore(img *html.Node, src string) int {
	text := strings.ToLower(strings.Join([]string{
		imageAttr(img, "alt"),
		imageAttr(img, "class"),
		imageAttr(img, "id"),
		imageAttr(img, "title"),
		src,
	}, " "))
	for _, bad := range []string{"logo", "avatar", "icon", "smiley", "spacer", "emoji", "badge"} {
		if strings.Contains(text, bad) {
			return 0
		}
	}

	score := 0
	for _, hint := range []string{"poster", "cover", "постер", "обложка"} {
		if strings.Contains(text, hint) {
			score += 10
			break
		}
	}
	if strings.Contains(strings.ToLower(src), "poster") || strings.Contains(strings.ToLower(src), "cover") {
		score += 2
	}
	width, _ := strconv.Atoi(imageAttr(img, "width"))
	height, _ := strconv.Atoi(imageAttr(img, "height"))
	if width >= 300 && height >= 300 {
		score += 3
	}
	if height > width && height > 0 {
		score++
	}
	return score
}

func firstImageAttr(img *html.Node, keys ...string) string {
	for _, key := range keys {
		if value := imageAttr(img, key); value != "" {
			return value
		}
	}
	return ""
}

func imageAttr(img *html.Node, key string) string {
	for i := range img.Attr {
		if strings.EqualFold(img.Attr[i].Key, key) {
			return strings.TrimSpace(img.Attr[i].Val)
		}
	}
	return ""
}

func resolvePosterURL(base, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if !ref.IsAbs() && base != "" {
		baseURL, err := url.Parse(base)
		if err != nil {
			return ""
		}
		ref = baseURL.ResolveReference(ref)
	}
	resolved := ref.String()
	if !safeHTTPURL(resolved) {
		return ""
	}
	return resolved
}

// requestErrorDetail strips the URL from net/http errors. Torrent and poster
// links can contain tracker passkeys, so only the underlying cause is returned.
func requestErrorDetail(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err
	}
	return err
}

// SafeHTTPURL reports whether raw is an absolute http(s) URL. UI code uses it
// before putting a tracker-supplied link into an href.
func SafeHTTPURL(raw string) bool {
	return safeHTTPURL(raw)
}

// safeHTTPURL reports whether raw is an absolute http(s) URL. It is used for
// external links from tracker results, so javascript:, data: and file: must
// never reach an href or an HTTP client.
func safeHTTPURL(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}

// SafeTorrentName turns a tracker title into a safe attachment filename:
// CR/LF, quotes, slashes and control characters are removed, the result is
// limited to 150 characters and an empty name becomes "torrent".
func SafeTorrentName(title string) string {
	title = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		switch r {
		case '"', '\'', '\\', '/', ':', '*', '?', '<', '>', '|':
			return -1
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, title)
	title = strings.TrimSpace(title)
	title = strings.Trim(title, ".")
	if title == "" {
		return "torrent"
	}
	runes := []rune(title)
	if len(runes) > 150 {
		runes = runes[:150]
	}
	return string(runes)
}
