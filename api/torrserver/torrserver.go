package torrserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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

const requestTimeout = 3 * time.Second

// ErrBadRequest marks a 400 response from Torrserver. Callers can map it to a
// user-facing "torrent unavailable" message instead of a generic outage.
var ErrBadRequest = errors.New("torrserver rejected the request")

// Request bodies are built as structs and serialized by encoding/json, never by
// string concatenation: titles, links and posters come from torrent trackers and
// scraped pages, so a quote or a backslash in them used to produce malformed
// JSON, and a crafted title could even inject extra fields into the request.
type listRequest struct {
	Action string `json:"action"`
}

type addRequest struct {
	Action   string `json:"action"`
	Link     string `json:"link"`
	Title    string `json:"title"`
	Poster   string `json:"poster"`
	SaveToDB bool   `json:"save_to_db"`
}

type remRequest struct {
	Action string `json:"action"`
	Hash   string `json:"hash"`
}

// endpoint is resolved on every call, not at package initialization: the config
// is loaded from main(), so anything captured during init would be empty.
func endpoint() string {
	return "http://" + common.Settings.Torrserver.Host + ":" +
		strconv.Itoa(common.Settings.Torrserver.Port) + "/torrents"
}

// post sends a request to Torrserver and returns the raw response body.
// The timeout is applied to the request context, so an expired deadline really
// cancels the HTTP call instead of merely abandoning a goroutine that keeps it
// running, and the body is always closed.
func post(parent context.Context, payload any, timeout time.Duration) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	// The body is read before the status check so that an error message from
	// Torrserver ends up in the returned error instead of being discarded.
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		detail := strings.TrimSpace(string(data))
		var err error
		if detail == "" {
			err = fmt.Errorf("request error: %s", res.Status)
		} else {
			err = fmt.Errorf("request error: %s: %s", res.Status, detail)
		}
		if res.StatusCode == http.StatusBadRequest {
			return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
		}
		return nil, err
	}
	return data, nil
}

func ListContext(ctx context.Context) (*[]TSListItem, error) {
	data, err := post(ctx, listRequest{Action: "list"}, requestTimeout)
	if err != nil {
		return nil, err
	}

	var list []TSListItem
	if err = json.Unmarshal(data, &list); err != nil {
		return nil, err
	}

	for i := range list {
		// "data" is custom and may be arbitrary text rather than JSON, so a
		// single odd entry must not fail the whole listing.
		if err := json.Unmarshal([]byte(list[i].Data), &list[i].DataStruct); err != nil {
			continue
		}
	}
	return &list, nil
}

func List() (*[]TSListItem, error) {
	return ListContext(context.Background())
}

func AddContext(ctx context.Context, link string, title string, poster string) error {
	_, err := post(ctx, addRequest{
		Action:   "add",
		Link:     link,
		Title:    title,
		Poster:   poster,
		SaveToDB: true,
	}, requestTimeout)
	return err
}

func Add(link string, title string, poster string) error {
	return AddContext(context.Background(), link, title, poster)
}

func DeleteContext(ctx context.Context, hash string) error {
	_, err := post(ctx, remRequest{Action: "rem", Hash: hash}, requestTimeout)
	return err
}

func Delete(hash string) error {
	return DeleteContext(context.Background(), hash)
}
