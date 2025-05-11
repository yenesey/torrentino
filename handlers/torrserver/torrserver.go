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
func NewPaginator() *TorrserverPaginator {
	var p TorrserverPaginator
	p = TorrserverPaginator{
		*paginator.New(&p, "torrserver", 4),
	}
	return &p
}

func (p *TorrserverPaginator) Item(i int) *torrserver.TSListItem {
	return p.Paginator.Item(i).(*torrserver.TSListItem)
}

// method overload
func (p *TorrserverPaginator) ItemString(i int) string {
	item := p.Item(i)
	return item.Title +
		" [" + utils.FormatFileSize(uint64(item.Torrent_Size)) + "]"
}

// method overload
func (p *TorrserverPaginator) ItemContextActions(i int) []string {
	return []string{"delete"}
}

// method overload
func (p *TorrserverPaginator) ItemActionExec(i int, actionKey string) bool {
	item := p.Item(i)
	if actionKey == "delete" {
		if err := torrserver.Delete(item.Hash); err == nil {
			p.Delete(i)
		} else {
			utils.LogError(err)
		}
	}
	return true
}

// method overload
func (p *TorrserverPaginator) LessItem(i int, j int, attributeKey string) bool {
	a := p.Item(i)
	b := p.Item(j)
	switch attributeKey {
	case "Size":
		return a.Torrent_Size < b.Torrent_Size
	}
	return false
}

// method overload
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

	return p.Paginator.Reload()
}

// -------------------------------------------------------------------------
func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	var p = NewPaginator()
	p.Sorting.Setup([]paginator.SortHeader{
		{AttributeName: "Size", ButtonText: "size", Order: 1},
	})
	p.Reload()
	p.Show(ctx, b, update.Message.Chat.ID)
}
