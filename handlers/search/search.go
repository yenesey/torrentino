package search

import (
	"context"
	"net/http"
	"strconv"

	"github.com/antchfx/htmlquery"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	gotorrentparser "github.com/j-muller/go-torrent-parser"
	"github.com/pkg/errors"
	"golang.org/x/net/html"

	"torrentino/api/jackett"
	"torrentino/api/torrserver"
	"torrentino/api/transmission"
	"torrentino/common"
	"torrentino/common/paginator"
	"torrentino/common/utils"
)

type ListItem struct {
	jackett.Result
	InTorrents   bool
	InTorrserver bool
}

type FindPaginator struct {
	paginator.Paginator
	query              string
	transmissionHashes map[string]bool
	torrserverHashes   map[string]bool
}

// ----------------------------------------
func NewPaginator(query string) *FindPaginator {
	var fp FindPaginator
	fp = FindPaginator{
		*paginator.New(&fp, "find", 4),
		query,
		make(map[string]bool),
		make(map[string]bool),
	}
	return &fp
}

func (p *FindPaginator) Item(i int) *ListItem {
	return p.Paginator.Item(i).(*ListItem)
}

// method overload
func (p *FindPaginator) ItemString(i int) string {

	item := p.Item(i)
	return item.Title +
		" [" + utils.FormatFileSize(uint64(item.Size)) + "] [" + item.TrackerId + "]" +
		" [" + strconv.Itoa(int(item.Seeders)) + "s/" + strconv.Itoa(int(item.Peers)) + "p]" +
		(func() string {
			if item.Link != "" {
				return " 📎"
			}
			return ""
		})() +
		(func() string {
			if item.MagnetUri != "" {
				return " 🧲"
			}
			return ""
		})() +
		(func() (result string) {
			if item.InTorrents {
				result += " 📥"
			}
			if item.InTorrserver {
				result += " 🎦"
			}
			return
		})()
}

// method overload
func (p *FindPaginator) AttributeByName(i int, attributeName string) string {
	item := p.Item(i)
	if attributeName == "TrackerId" {
		return item.TrackerId
	} else if attributeName == "TrackerType" {
		return item.TrackerType
	}
	return ""
}

// method overload
func (p *FindPaginator) LessItem(i int, j int, attributeKey string) bool {
	a := p.Item(i)
	b := p.Item(j)
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
func (p *FindPaginator) ItemActions(i int) (result []string) {

	item := p.Item(i)
	if item.InfoHash == "" && item.Link != "" {
		res, err := http.Get(item.Link)
		if err != nil {
			utils.LogError(errors.Wrap(err, "ItemActions: http.Get"))
		} else {
			torrent, err := gotorrentparser.Parse(res.Body)
			if err != nil {
				utils.LogError(errors.Wrap(err, "ItemActions: gotorrentparser.Parse"))
			}
			item.InfoHash = torrent.InfoHash
		}
	}

	if p.transmissionHashes[item.InfoHash] {
		item.InTorrents = true
	}
	if p.torrserverHashes[item.InfoHash] {
		item.InTorrserver = true
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
func (p *FindPaginator) ItemActionExec(i int, actionKey string) (unselectItem bool) {

	item := p.Item(i)

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
			utils.LogError(errors.Wrap(err, "ItemActionExec"))
		} else {
			item.InTorrents = true
		}

	case "torrsrv":
		err := torrserver.Add(urlOrMagnet, item.Title, getPosterLinkFromPage(item.Details))
		if err != nil {
			utils.LogError(errors.Wrap(err, "ItemActionExec"))
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
			utils.LogError(errors.Wrap(err, "ItemActionExec"))
			return false
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

// method overload
func (p *FindPaginator) Reload() {

	result, err := jackett.Query(p.query, common.Settings.Jackett.Indexers)
	if err != nil {
		utils.LogError(errors.Wrap(err, "Reload: jackett.Query()"))
		return
	}

	trList, err := transmission.List()
	if err != nil {
		utils.LogError(errors.Wrap(err, "Reload: transmission.List()"))
	} else {
		for _, el := range *trList {
			p.transmissionHashes[*el.HashString] = true
		}
	}

	tsList, err := torrserver.List()
	if err != nil {
		utils.LogError(errors.Wrap(err, "Reload: torrserver.List()"))
	} else {
		for _, el := range *tsList {
			p.torrserverHashes[el.Hash] = true
		}
	}

	p.Alloc(len(*result))
	for i := range *result {
		hash := (*result)[i].InfoHash
		p.Append(&ListItem{(*result)[i], p.transmissionHashes[hash], p.torrserverHashes[hash]})
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
		utils.LogError(errors.Wrap(err, "getPosterLinkFromPage: htmlquery.LoadURL"))
		return ""
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
		{Name: "Link", ShortName: "file", Order: 0},
	})
	p.Filtering.Setup([]string{"TrackerId"})
	p.Reload()
	p.Show(ctx, b, update.Message.Chat.ID)
}
