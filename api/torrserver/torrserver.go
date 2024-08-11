package torrserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"torrentino/common"
)

type TSListItem struct {
	Title        string
	Torrent_Size int64
	Hash         string
	Data         string
	DataStruct   struct {
		Torrserver struct {
			Files []struct {
				Id     int
				Path   string
				Length int64
			}
		}
	}
}

var url = "http://" + common.Settings.Torrserver.Host + ":" + strconv.Itoa(common.Settings.Torrserver.Port) + "/torrents"

/*
   {
   "action": "add/get/set/rem/list/drop",
   "link": "hash/magnet/link to torrent",
   "hash": "hash of torrent",
   "title": "title of torrent",
   "poster": "link to poster of torrent",
   "data": "custom data of torrent, may be json",
   "save_to_db": true/false
   }
*/

func List() (*[]TSListItem, error) {

	res, err := http.Post(
		url,
		"application/json",
		strings.NewReader("{\"action\" : \"list\"}"),
	)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var List []TSListItem
	err = json.Unmarshal(data, &List)
	if err != nil {
		return nil, err
	}

	for i := range List {
		err = json.Unmarshal([]byte(List[i].Data), &List[i].DataStruct)
		if err != nil {
			return nil, err
		}
	}
	return &List, nil
}

func Add(link string, title string, poster string) error {
	res, err := http.Post(
		url,
		"application/json",
		strings.NewReader("{\"action\" : \"add\","+
			"\"link\" : \""+link+"\","+
			"\"title\" : \""+title+"\","+
			"\"poster\" : \""+poster+"\","+
			"\"save_to_db\" : true}"),
	)
	if res.StatusCode != 200 {
		return fmt.Errorf("request error: %s", res.Status)
	}
	return err
}

func Delete(hash string) error {
	res, err := http.Post(
		url,
		"application/json",
		strings.NewReader(
			"{\"action\" : \"rem\","+
				"\"hash\" : \""+hash+"\"}"),
	)
	if res.StatusCode != 200 {
		return fmt.Errorf("request error: %s", res.Status)
	}
	return err
}
