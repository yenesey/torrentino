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
	"github.com/pkg/errors"
	"golang.org/x/net/html"
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
	//todo: 1 get torrent_list
	//todo: 2 check each list[i] item is already downloading...
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
					result += " [->downloading]"
				}
				if data.InTorrserver {
					result += " [->torrserver]"
				}
				return
			})()

	} else {
		logError(fmt.Errorf("ItemString %s", "error"))
	}
	return ""
}

// method overload
func (p *FindPaginator) KeepItem(item_ any, attributeKey string, attributeValue string) bool {
	item := item_.(*ListItem)

	if attributeKey == "TrackerId" {
		if item.TrackerId == attributeValue {
			return true
		}
	} else if attributeKey == "TrackerType" {
		if item.TrackerType == attributeValue {
			return true
		}
	}

	return false
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
		return a.Link < b.Link
	}
	return false
}

// method overload
func (p *FindPaginator) ItemActions(i int) (result []string) {

	item := p.Item(i).(*ListItem)
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
func (p *FindPaginator) ItemActionExec(i int, actionKey string) bool {

	item := p.Item(i).(*ListItem)
	
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
		}
		item.InTorrents = true
	
	case "torrsrv":
		err := torrserver.Add(item.MagnetUri, item.Title, getPosterLinkFromPage(item.Details))
		if err != nil {
			logError(errors.Wrap(err, "ItemActionExec"))
		}
		item.InTorrserver = true

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

// method overload
func (p *FindPaginator) Reload() {

	var result, err = jackett.Query(p.query, nil)
	if err != nil {
		logError(errors.Wrap(err, "Reload"))
	}
	
	p.Alloc(len(result.Results))
	for i := range result.Results {
		p.Append(&ListItem{result.Results[i], false, false})
	}
	p.Paginator.Reload()
}

//-------------------------------------------------------------------------
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
		logError(errors.Wrap(err, "getPosterLinkFromPage"))
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
