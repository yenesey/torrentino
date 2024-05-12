package torrserver

import (
	"encoding/json"
	"io"

	"log"

	"net/http"
	// "net/http/cookiejar"
	// "net/url"
	"strconv"
	// "time"
	"strings"
	// "github.com/pkg/errors"
	"torrentino/common"
)

type TSListItem struct {
	Title        string
	Torrent_Size int64
	Hash         string
}

type TSList []TSListItem

func ListItems() (*TSList, error) {
	ts := &common.Settings.Torrserver
	client := &http.Client{}
	req, err := http.NewRequest("POST", "http://"+ts.Host+":"+strconv.Itoa(ts.Port) + "/torrents", strings.NewReader("{\"action\" : \"list\"}"))
	if err != nil {
		log.Fatal(err)
	}
	res, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}
	var List TSList
	err = json.Unmarshal(data, &List)
	if err != nil {
		log.Fatal(err)
	}
	return &List, nil
}

func init() {
}
