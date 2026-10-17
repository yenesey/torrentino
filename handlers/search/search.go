package search

import (
	"bytes"
	"context"
	"strconv"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"torrentino/common/paginator"
	"torrentino/common/utils"
	"torrentino/core"
)

type FindPaginator struct {
	paginator.Paginator
	ctx     context.Context
	service *core.Service
	query   string
}

// ----------------------------------------
func NewPaginator(ctx context.Context, b *bot.Bot, update *models.Update) *FindPaginator {
	var p FindPaginator
	p.Paginator = *paginator.New(ctx, b, update, "find", 4, &p, &p, &p)
	p.ctx = ctx
	p.service = core.Default
	if update != nil && update.Message != nil {
		p.query = update.Message.Text
	}
	return &p
}

func (p *FindPaginator) Item(i int) *core.FindItem {
	return p.Paginator.Item(i).(*core.FindItem)
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
	return core.FindCompare(p.Item(i), p.Item(j), attribute)
}

// method overload
func (p *FindPaginator) Actions(i int) (result []string) {
	item := p.Item(i)

	if item.InfoHash == "" && item.Link != "" {
		if err := p.service.ResolveHash(p.ctx, item); err != nil {
			utils.LogError(err)
		}
	}
	return core.ActionIDs(p.service.FindActions(item))
}

// method overload
func (p *FindPaginator) Execute(i int, action string) (unselect bool) {
	item := p.Item(i)

	switch action {
	case "web page":
		p.ReplyMessage(item.Details)
		return true
	case ".torrent":
		data, err := p.service.TorrentFile(p.ctx, item.Link)
		if err != nil {
			utils.LogError(err)
			return false
		}
		p.ReplyDocument(&models.InputFileUpload{
			Filename: core.SafeTorrentName(item.Title) + ".torrent",
			Data:     bytes.NewReader(data),
		})
		return true
	}

	if err := p.service.DoFind(p.ctx, item, action); err != nil {
		utils.LogError(err)
		return false
	}
	return true
}

func (p *FindPaginator) Reload() error {
	items, err := p.service.Search(p.ctx, p.query)
	if err != nil {
		utils.LogError(err)
		return err
	}

	p.Alloc(len(items))
	for _, item := range items {
		p.Append(item)
	}
	return nil
}

func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	var p = NewPaginator(ctx, b, update)
	p.SetupSorting(core.FindSorting())
	p.SetupFiltering(core.FindFilters())
	if err := p.Reload(); err != nil {
		p.ReplyMessage(err.Error())
	} else {
		p.Show()
	}
}
