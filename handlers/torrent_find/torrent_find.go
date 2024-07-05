package torrent_find

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"torrentino/api/jackett"
	"torrentino/api/torrserver"
	"torrentino/api/transmission"
	"torrentino/common/paginator"
	"torrentino/common/utils"

	"github.com/antchfx/htmlquery"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/hekmon/transmissionrpc/v2"
	"github.com/pkg/errors"
	"golang.org/x/net/html"
	"github.com/j-muller/go-torrent-parser" 
)

type ListItem struct {
	jackett.Result
	InTorrents   bool
	InTorrserver bool
}

type FindPaginator struct {
	paginator.Paginator
	query string
}

var transmissionHashes map[string]bool
var torrserverHashes map[string]bool

// ----------------------------------------
func logError(err error) {
	log.Printf("[handlers/torrent_find] %s", err)
}

// ----------------------------------------
func NewPaginator(query string) *FindPaginator {
	var fp FindPaginator
	fp = FindPaginator{
		*paginator.New(&fp, "find", 4),
		query,
	}
	return &fp
}

// method overload
func (p *FindPaginator) ItemString(item any) string {

	if data, ok := item.(*ListItem); ok {
		return data.Title +
			" [" + utils.FormatFileSize(uint64(data.Size)) + "] [" + data.TrackerId + "]" +
			" [" + strconv.Itoa(int(data.Seeders)) + "s/" + strconv.Itoa(int(data.Peers)) + "p]" +
			(func() (result string) {
				if data.InTorrents {
					result += " [->downloads]"
				}
				if data.InTorrserver {
					result += " [->torrserver]"
				}
				return
			})() +
			(func() string {
				if data.Link != "" {
					return " [L:]"
				}
				return ""
			})()

	} else {
		logError(fmt.Errorf("ItemString: type assertion error"))
	}
	return ""
}

// method overload
func (p *FindPaginator) AttributeByName(item any, attributeName string) string {
	item_ := item.(*ListItem)
	if attributeName == "TrackerId" {
		return item_.TrackerId
	} else if attributeName == "TrackerType" {
		return item_.TrackerType
	}
	return ""
}

// method overload
func (p *FindPaginator) LessItem(i int, j int, attributeKey string) bool {
	a := p.Item(i).(*ListItem)
	b := p.Item(j).(*ListItem)
	switch attributeKey {
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

// method overload
func (p *FindPaginator) ItemActions(item_ any) (result []string) {

	item := item_.(*ListItem)
	if item.InfoHash == "" {
		res, err := http.Get(item.Link)
		if err != nil {
			logError(errors.Wrap(err, "ItemActions: http.Get"))
		}
		torrent, err := gotorrentparser.Parse(res.Body)
		if err != nil {
			logError(errors.Wrap(err, "ItemActions: gotorrentparser.Parse"))
		}
		item.InfoHash = torrent.InfoHash
		if transmissionHashes[item.InfoHash] {
			item.InTorrents = true
		}
		if torrserverHashes[item.InfoHash] {
			item.InTorrserver = true
		}
	}

	if !item.InTorrents {
		result = append(result, "download")
	}
	if !item.InTorrserver {
		result = append(result, "torrsrv")
	}
	if item.Link != "" {
		result = append(result, ".torrent")
	}
	if item.Details != "" {
		result = append(result, "web page")
	}
	return result
}

// method overload
func (p *FindPaginator) ItemActionExec(item_ any, actionKey string) (unselectItem bool) {

	item := item_.(*ListItem)

	var urlOrMagnet string
	if item.Link != "" {
		urlOrMagnet = item.Link
	} else {
		urlOrMagnet = item.MagnetUri
	}

	switch actionKey {
	case "download":
		_, err := transmission.Add(urlOrMagnet)
		if err != nil {
			logError(errors.Wrap(err, "ItemActionExec"))
		} else {
			item.InTorrents = true
		}

	case "torrsrv":
		err := torrserver.Add(urlOrMagnet, item.Title, getPosterLinkFromPage(item.Details))
		if err != nil {
			logError(errors.Wrap(err, "ItemActionExec"))
		} else {
			item.InTorrserver = true
		}
	case "web page":
		p.Bot.SendMessage(p.Ctx, &bot.SendMessageParams{
			ChatID:      p.ChatID,
			Text:        item.Details,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: nil,
		})

	case ".torrent":
		res, err := http.Get(item.Link)
		if err != nil {
			logError(errors.Wrap(err, "ItemActionExec"))
		}
		p.Bot.SendDocument(p.Ctx, &bot.SendDocumentParams{
			ChatID:      p.ChatID,
			Document:    &models.InputFileUpload{Filename: item.Title + ".torrent", Data: res.Body},
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: nil,
		})
	}

	return true
}

func mapIt[T any](listFunc func() (*[]T, error), attrValueFunc func(*T) string) (result map[string]bool) {
	result = make(map[string]bool)
	list, err := listFunc()
	if err != nil {
		logError(errors.Wrap(err, "mapIt"))
		return
	}
	for i := range *list {
		result[attrValueFunc(&(*list)[i])] = true
	}
	return
}

// method overload
func (p *FindPaginator) Reload() {

	result, err := jackett.Query(p.query, nil)
	if err != nil {
		logError(errors.Wrap(err, "Reload: jackett.Query"))
	}

	transmissionHashes = mapIt[transmissionrpc.Torrent](
		transmission.List,
		func(el *transmissionrpc.Torrent) string {
			return *el.HashString
		},
	)
	torrserverHashes = mapIt[torrserver.TSListItem](
		torrserver.List,
		func(el *torrserver.TSListItem) string {
			return el.Hash
		},
	)

	p.Alloc(len(*result))
	for i := range *result {
		hash := (*result)[i].InfoHash
		p.Append(&ListItem{(*result)[i], transmissionHashes[hash], torrserverHashes[hash]})
	}
	p.Paginator.Reload()
}

// -------------------------------------------------------------------------
func getPosterLinkFromPage(url string) string {

	var findKey = func(attr []html.Attribute, key string) string {
		for i := range attr {
			if attr[i].Key == key {
				return attr[i].Val
			}
		}
		return ""
	}

	doc, err := htmlquery.LoadURL(url)
	if err != nil {
		logError(errors.Wrap(err, "getPosterLinkFromPage: htmlquery.LoadURL"))
	}

	poster := htmlquery.Find(doc, "//var[@class=\"postImg postImgAligned img-right\"]") // rutracker
	if len(poster) > 0 {
		if res := findKey(poster[0].Attr, "title"); res != "" {
			return res
		}
	}
	poster = htmlquery.Find(doc, "//table[@id=\"details\"]/tr/td[2]/img") // rutor
	if len(poster) > 0 {
		if res := findKey(poster[0].Attr, "src"); res != "" {
			return res
		}
	}
	poster = htmlquery.Find(doc, "//table[@id=\"details\"]//img")
	if len(poster) > 0 {
		if res := findKey(poster[0].Attr, "src"); res != "" {
			return res
		}
	}
	return ""
}

func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	var p = NewPaginator(update.Message.Text)
	p.Sorting.Setup([]paginator.SortHeader{
		{Name: "Size", ShortName: "size", Order: 1},
		{Name: "Seeders", ShortName: "seeds", Order: 1},
		{Name: "Peers", ShortName: "peers", Order: 0},
		{Name: "Link", ShortName: "link", Order: 0},
	})
	p.Filtering.Setup([]string{"TrackerId"})
	p.Reload()
	p.Show(ctx, b, update.Message.Chat.ID)
}
