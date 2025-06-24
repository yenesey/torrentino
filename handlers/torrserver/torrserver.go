package torrserver

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"torrentino/api/torrserver"
	"torrentino/common/paginator"
	"torrentino/common/utils"
)

type TorrserverPaginator struct {
	paginator.Paginator
}

// ----------------------------------------
func NewPaginator(ctx context.Context, b *bot.Bot, update *models.Update) *TorrserverPaginator {
	var p TorrserverPaginator
	p = TorrserverPaginator{
		*paginator.New(ctx, b, update, "torrserver", 4, &p, &p, &p),
	}
	return &p
}

func (p *TorrserverPaginator) Item(i int) *torrserver.TSListItem {
	return p.Paginator.Item(i).(*torrserver.TSListItem)
}

// method overload
func (p *TorrserverPaginator) Line(i int) string {
	item := p.Item(i)
	return item.Title +
		" [" + utils.FormatFileSize(uint64(item.Torrent_Size)) + "]"
}

// method overload
func (p *TorrserverPaginator) Actions(i int) []string {
	return []string{"delete"}
}

// method overload
func (p *TorrserverPaginator) Execute(i int, action string) (unselect bool) {
	item := p.Item(i)
	if action == "delete" {
		if err := torrserver.Delete(item.Hash); err == nil {
			p.Delete(i)
		} else {
			utils.LogError(err)
		}
	}
	return true
}

// method overload
func (p *TorrserverPaginator) Stringify(i int, attribute string) string {
	return ""
}

// method overload
func (p *TorrserverPaginator) Compare(i int, j int, attribute string) bool {
	a := p.Item(i)
	b := p.Item(j)
	switch attribute {
	case "Size":
		return a.Torrent_Size < b.Torrent_Size
	}
	return false
}

func (p *TorrserverPaginator) Reload() error {

	result, err := torrserver.List()
	if err != nil {
		utils.LogError(err)
		return err
	}

	p.Alloc(len(*result))
	for i := range *result {
		p.Append(&(*result)[i])
	}
	return nil
}

// -------------------------------------------------------------------------
func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	var p = NewPaginator(ctx, b, update)
	p.SetupSorting([]paginator.Sorting{
		{Attribute: "Size", Alias: "size", Order: 1},
	})
	if err := p.Reload(); err != nil {
		p.ReplyMessage(err.Error())
	} else {
		p.Show()
	}
}
