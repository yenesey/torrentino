package torrent_find

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"torrentino/api/jackett"
	"torrentino/api/transmission"
	"torrentino/common/paginator"
	"torrentino/common/utils"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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

func NewPaginator(query string) *FindPaginator {
	var fp FindPaginator
	fp = FindPaginator{
		*paginator.New(&fp, "find", 4),
		query,
	}
	//todo: 1 get torrent_list
	//todo: 2 check each list[i] item is already downloading...
	fp.Reload()
	return &fp
}

// method overload
func (p *FindPaginator) ItemString(item any) string {

	if data, ok := item.(*ListItem); ok {
		return data.Title +
			" [" + utils.FormatFileSize(int64(data.Size), 1024.0) + "] [" + data.TrackerId + "]" +
			" [" + strconv.Itoa(int(data.Seeders)) + "s/" + strconv.Itoa(int(data.Peers)) + "p]" +
			(func() string {
				if data.InTorrents {
					return " [downloading]"
				}
				return ""
			})()

	} else {
		log.Fatalf("ItemString")
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

	if actionKey == "download" {
		var urlOrMagnet string
		if item.Link != "" {
			urlOrMagnet = item.Link
		} else {
			urlOrMagnet = item.MagnetUri
		}
		_, err := transmission.Add(urlOrMagnet)
		if err != nil {
			log.Fatal(err)
		}
		item.InTorrents = true

	} else if actionKey == "web page" {
		p.Bot.SendMessage(p.Ctx, &bot.SendMessageParams{
			ChatID:      p.ChatID,
			Text:        item.Details,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: nil,
		})
	} else if actionKey == ".torrent" {
		client := &http.Client{}
		req, err := http.NewRequest("GET", item.Link, nil)
		if err != nil {
			log.Fatal(err)
		}
		res, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
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
		log.Fatal(err)
	}
	
	p.Alloc(len(result.Results))
	for i := range result.Results {
		p.Append(&ListItem{result.Results[i], false, false})
	}

}

//-------------------------------------------------------------------------
func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	var pg = NewPaginator(update.Message.Text)
	pg.Sorting.Setup([]paginator.SortHeader{
		{Name: "Size", ShortName: "size", Order: 1},
		{Name: "Seeders", ShortName: "seeds", Order: 1},
		{Name: "Peers", ShortName: "peers", Order: 0},
		{Name: "Link", ShortName: "link", Order: 0},
	})
	pg.Filtering.Setup([]string{"TrackerId"})
	pg.Show(ctx, b, update.Message.Chat.ID)
}