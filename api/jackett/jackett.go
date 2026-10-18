package jackett

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	gotorrentparser "github.com/j-muller/go-torrent-parser"
	"github.com/pkg/errors"

	"torrentino/common"
)

type jackettTime struct {
	time.Time
}

func (jt *jackettTime) UnmarshalJSON(b []byte) (err error) {
	str := strings.Trim(string(b), `"`)
	if str == "0001-01-01T00:00:00" {
	} else if len(str) == 19 {
		jt.Time, err = time.Parse(time.RFC3339, str+"Z")
	} else {
		jt.Time, err = time.Parse(time.RFC3339, str)
	}
	return
}

type Result struct {
	FirstSeen            jackettTime
	Tracker              string
	TrackerId            string
	TrackerType          string
	CategoryDesc         string
	BlackholeLink        string
	Title                string
	Guid                 string
	Link                 string
	Details              string
	PublishDate          jackettTime
	Category             []uint
	Size                 uint
	Files                uint
	Grabs                uint
	Description          string
	RageID               uint
	TVDBId               uint
	Imdb                 uint
	TMDb                 uint
	TVMazeId             uint
	TraktId              uint
	DoubanId             uint
	Genres               []string
	Languages            []string
	Subs                 []string
	Year                 uint
	Author               string
	BookTitle            string
	Publisher            string
	Artist               string
	Album                string
	Label                string
	Track                string
	Seeders              uint
	Peers                uint
	Poster               string
	InfoHash             string
	MagnetUri            string
	MinimumRatio         float32
	MinimumSeedTime      uint
	DownloadVolumeFactor float32
	UploadVolumeFactor   float32
	Gain                 float32
}

type Indexer struct {
	ID          string
	Name        string
	Description string
	Configured  bool
	Status      int
	Results     int
	Error       string
}

type QueryResults struct {
	Results  []Result
	Indexers []Indexer
}

var client *http.Client

// baseUrl and apiKey are resolved on every call rather than captured during
// package initialization: the config is loaded from main(), so values read in
// init() would be empty.
func baseUrl() string {
	return "http://" + common.Settings.Jackett.Host + ":" +
		strconv.Itoa(common.Settings.Jackett.Port) + "/api/v2.0/"
}

func apiKey() string {
	return common.Settings.Jackett.APIKey
}

// httpGet performs one GET request with a context deadline. The context is
// derived from the caller's context, so cancelling the HTTP request from the
// web (or the bot shutting down) also cancels the upstream call. The body is
// always closed, and the deadline is applied to the request itself rather than
// to a goroutine that would keep it running in the background.
func httpGet(ctx context.Context, url string, timeout time.Duration) (*[]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("request timeout: %v", requestErrorDetail(err))
		}
		return nil, fmt.Errorf("request failed: %v", requestErrorDetail(err))
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Request error: %s", res.Status)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

// requestErrorDetail removes the URL from net/http errors: the Jackett URL
// contains the API key, and those errors are logged.
func requestErrorDetail(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err
	}
	return err
}

func GetValidIndexersContext(ctx context.Context) (*[]Indexer, error) {
	var r []Indexer
	data, err := httpGet(ctx, baseUrl()+"indexers?Configured=true", common.Settings.JackettTimeout())
	if err != nil {
		return nil, errors.Wrap(err, "GetValidIndexers")
	}
	err = json.Unmarshal(*data, &r)
	if err != nil {
		return nil, errors.Wrap(err, "GetValidIndexers")
	}
	return &r, nil
}

func GetValidIndexers() (*[]Indexer, error) {
	return GetValidIndexersContext(context.Background())
}

func GetInfoHashContext(ctx context.Context, url string) (string, error) {
	res, err := httpGet(ctx, url, 5*time.Second)
	if err != nil {
		return "", err
	}
	torrent, err := gotorrentparser.Parse(bytes.NewReader(*res))
	if err != nil {
		return "", err
	}
	return torrent.InfoHash, nil
}

func GetInfoHash(url string) (string, error) {
	return GetInfoHashContext(context.Background(), url)
}

func QueryContext(ctx context.Context, str string, indexers []string) (*[]Result, error) {

	var u = "indexers/status:healthy,test:passed/results?apikey=" + apiKey()
	for _, indexer := range indexers {
		u = u + "&Tracker[]=" + indexer
	}
	u = u + "&Query=" + url.QueryEscape(str)
	// The deadline is configurable ("jackett.timeout", 60s by default) because
	// Jackett queries every configured indexer sequentially, and a broad search
	// over slow trackers easily outlives a single-request timeout.
	data, err := httpGet(ctx, baseUrl()+u, common.Settings.JackettTimeout())
	if err != nil {
		return nil, errors.Wrap(err, "Jackett")
	}

	var r QueryResults
	err = json.Unmarshal(*data, &r)
	if err != nil {
		return nil, errors.Wrap(err, "Jackett")
	}
	return &r.Results, nil
}

func Query(str string, indexers []string) (*[]Result, error) {
	return QueryContext(context.Background(), str, indexers)
}

func init() {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("error creating cookie jar %s", err.Error())
	}
	client = &http.Client{
		Jar: jar,
	}
}
