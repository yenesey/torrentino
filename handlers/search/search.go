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
func NewPaginator(ctx context.Context, b *bot.Bot, update *models.Update) *FindPaginator {
	var p FindPaginator
	p = FindPaginator{
		*paginator.New(ctx, b, update, "find", 4, &p, &p, &p),
		update.Message.Text,
		make(map[string]bool),
		make(map[string]bool),
	}
	return &p
}

func (p *FindPaginator) Item(i int) *ListItem {
	return p.Paginator.Item(i).(*ListItem)
}

// method overload
func (p *FindPaginator) Line(i int) string {

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
func (p *FindPaginator) Stringify(i int, attributeName string) string {
	item := p.Item(i)
	switch attributeName {
	case "TrackerId":
		return item.TrackerId
	case "TrackerType":
		return item.TrackerType
	}
	return ""
}

// method overload
func (p *FindPaginator) Compare(i int, j int, attribute string) bool {
	a := p.Item(i)
	b := p.Item(j)
	switch attribute {
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
func (p *FindPaginator) Actions(i int) (result []string) {

	item := p.Item(i)
	if item.InfoHash == "" && item.Link != "" {
		res, err := http.Get(item.Link)
		if err != nil {
			utils.LogError(err)
		} else if res.StatusCode != 200 {
			utils.LogError(errors.New(res.Status + " (request Jackett by item url)"))
		} else {
			torrent, err := gotorrentparser.Parse(res.Body)
			if err != nil {
				utils.LogError(err)
			} else {
				item.InfoHash = torrent.InfoHash
			}
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
func (p *FindPaginator) Execute(i int, action string) (unselect bool) {

	item := p.Item(i)

	var urlOrMagnet string
	if item.Link != "" {
		urlOrMagnet = item.Link
	} else {
		urlOrMagnet = item.MagnetUri
	}

	var err error
	switch action {
	case "download":
		if _, err = transmission.Add(urlOrMagnet); err == nil {
			item.InTorrents = true
		}

	case "torrsrv":
		if err = torrserver.Add(urlOrMagnet, item.Title, getPosterLinkFromPage(item.Details, item.TrackerId)); err == nil {
			item.InTorrserver = true
		}
	case "web page":
		p.ReplyMessage(item.Details)

	case ".torrent":
		var res *http.Response
		if res, err = http.Get(item.Link); err == nil {
			p.ReplyDocument(&models.InputFileUpload{Filename: item.Title + ".torrent", Data: res.Body})
		}
	}
	if err != nil {
		utils.LogError(err)
		return false
	}
	return true
}

func (p *FindPaginator) Reload() error {

	result, err := jackett.Query(p.query, common.Settings.Jackett.Indexers)
	if err != nil {
		utils.LogError(err)
		return err
	}

	trList, err := transmission.List()
	if err != nil {
		utils.LogError(err)
	} else {
		for _, el := range *trList {
			p.transmissionHashes[*el.HashString] = true
		}
	}

	tsList, err := torrserver.List()
	if err != nil {
		utils.LogError(err)
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
	return nil

}

// -------------------------------------------------------------------------
func getPosterLinkFromPage(pageUrl string, tracker string) string {

	var findKey = func(attr []html.Attribute, key string) string {
		for i := range attr {
			if attr[i].Key == key {
				return attr[i].Val
			}
		}
		return ""
	}
	// _, err := url.Parse(pageUrl)
	// if err != nil {
	// 	utils.LogError(err)
	// 	return ""
	// }

	doc, err := htmlquery.LoadURL(pageUrl)
	if err != nil {
		utils.LogError(err)
		return ""
	}

	switch tracker {
	case "rutor":
		poster := htmlquery.Find(doc, "//*[@id=\"details\"]/*/tr[1]/td[2]/img")
		if len(poster) > 0 {
			if res := findKey(poster[0].Attr, "src"); res != "" {
				return res
			}
		}
	case "rutracker":
		poster := htmlquery.Find(doc, "//var[@class=\"postImg postImgAligned img-right\"]")
		if len(poster) > 0 {
			if res := findKey(poster[0].Attr, "title"); res != "" {
				return res
			}
		}
	case "kinozal":
		poster := htmlquery.Find(doc, "//*[@id=\"main\"]/div[2]/div[1]/div[2]/ul/li[1]/a/img")
		if len(poster) > 0 {
			if res := findKey(poster[0].Attr, "src"); res != "" {
				return res
			}
		}
	}

	return ""
}

func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	var p = NewPaginator(ctx, b, update)
	p.SetupSorting([]paginator.Sorting{
		{Attribute: "Size", Alias: "size", Order: 1},
		{Attribute: "Seeders", Alias: "seeds", Order: 1},
		{Attribute: "Peers", Alias: "peers", Order: 0},
		{Attribute: "Link", Alias: "file", Order: 0},
	})
	p.SetupFiltering([]string{"TrackerId"})
	if err := p.Reload(); err != nil {
		p.ReplyMessage(err.Error())
	} else {
		p.Show()
	}
}
