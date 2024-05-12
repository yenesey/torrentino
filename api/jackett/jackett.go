package jackett

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
	"torrentino/common"

	"github.com/pkg/errors"
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

func httpGet(addUrl string) (*[]byte, error) {
	req, err := http.NewRequest("GET", baseUrl + addUrl, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create http get request")
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to do http get")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("httpGet:" + res.Status)
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response http GET")
	}

	return &data, nil
}

func GetValidIndexers() (*[]Indexer, error) {
	var r []Indexer

	data, err := httpGet("indexers?Configured=true")
	if err != nil {
		return nil, errors.Wrap(err, "GetValidIndexers failed")
	}

	err = json.Unmarshal(*data, &r)
	if err != nil {
		return nil, errors.Wrap(err, "GetValidIndexers failed")
	}

	return &r, nil
}

func Query(str string, indexers []string) (*QueryResults, error) {
	//var url = j.url + "indexers/all/results?apikey=" + j.apiKey + "&Query=" + str
	var u = "indexers/status:healthy,test:passed/results?apikey=" + apiKey
	for _, indexer := range indexers {
		u = u + "&Tracker[]=" + indexer
	}
	u = u + "&Query=" + url.QueryEscape(str)
	data, err := httpGet(u)
	if err != nil {
		return nil, errors.Wrap(err, "failed query Jackett")
	}

	var r QueryResults
	err = json.Unmarshal(*data, &r)
	if err != nil {
		return nil, errors.Wrap(err, "failed query Jackett")
	}
	return &r, nil
}

/*
func (j *Jackett) CommonGet(url string) (*[]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create http get request")
	}
	res, err := j.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to do http get")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("httpGet:" + res.Status)
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response http GET")
	}

	return &data, nil
}
*/

func init() {
	var jkt = &common.Settings.Jackett
	apiKey = jkt.Api_key
	baseUrl = "http://" + jkt.Host + ":" + strconv.Itoa(jkt.Port) + "/api/v2.0/"
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("Got error while creating cookie jar %s", err.Error())
	}
	client = &http.Client{
		Jar: jar,
	}
}
