package jackett

import (
	"bytes"
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
	"torrentino/common/utils"
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

var apiKey string
var baseUrl string
var client *http.Client

func httpGet(url string, timeout time.Duration) (*[]byte, error) {
	var res *http.Response
	err := utils.WithTimeout(
		func() error {
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return err
			}
			res, err = client.Do(req)
			if err != nil {
				return err
			}
			if res != nil && res.StatusCode != 200 {
				return fmt.Errorf("Request error: %s", res.Status)
			}
			return err
		},
		timeout,
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func GetValidIndexers() (*[]Indexer, error) {
	var r []Indexer
	data, err := httpGet("indexers?Configured=true", 30*time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "GetValidIndexers")
	}
	err = json.Unmarshal(*data, &r)
	if err != nil {
		return nil, errors.Wrap(err, "GetValidIndexers")
	}
	return &r, nil
}

func GetInfoHash(url string) (string, error) {
	res, err := httpGet(url, 5*time.Second)
	if err != nil {
		return "", err
	}
	torrent, err := gotorrentparser.Parse(bytes.NewReader(*res))
	if err != nil {
		return "", err
	}
	return torrent.InfoHash, nil
}

func Query(str string, indexers []string) (*[]Result, error) {

	var u = "indexers/status:healthy,test:passed/results?apikey=" + apiKey
	for _, indexer := range indexers {
		u = u + "&Tracker[]=" + indexer
	}
	u = u + "&Query=" + url.QueryEscape(str)
	data, err := httpGet(baseUrl+u, 30*time.Second)
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

func init() {
	var jkt = &common.Settings.Jackett
	apiKey = jkt.APIKey
	baseUrl = "http://" + jkt.Host + ":" + strconv.Itoa(jkt.Port) + "/api/v2.0/"
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("error creating cookie jar %s", err.Error())
	}
	client = &http.Client{
		Jar: jar,
	}
}
