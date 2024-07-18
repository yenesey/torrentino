package torrserver

import (
	"context"
	"fmt"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/pkg/errors"
	"torrentino/api/torrserver"
	"torrentino/common/paginator"
	"torrentino/common/utils"
)

type TorrserverList struct {
	paginator.Paginator
}

// ----------------------------------------
func NewPaginator() *TorrserverList {
	var p TorrserverList
	p = TorrserverList{
		*paginator.New(&p, "torrerver", 4),
	}
	return &p
}

// method overload
func (p *TorrserverList) ItemString(item_ any) string {

	if item, ok := item_.(torrserver.TSListItem); ok {
		return item.Title +
			" [" + utils.FormatFileSize(uint64(item.Torrent_Size)) + "]"
	} else {
		utils.LogError(fmt.Errorf("ItemString %s", "error"))
	}
	return ""
}

// method overload
func (p *TorrserverList) ItemActions(item any) (result []string) {
	result = []string{"delete"}
	return
}

// method overload
func (p *TorrserverList) ItemActionExec(item_ any, actionKey string) bool {
	item := item_.(torrserver.TSListItem)
	if actionKey == "delete" {
		if err := torrserver.Delete(item.Hash); err == nil {
			p.Reload()
			p.Refresh()
		} else {
			utils.LogError(errors.Wrap(err, "ItemActionExec"))
		}
	}

	return true
}

// method overload
func (p *TorrserverList) LessItem(i int, j int, attributeKey string) bool {
	a := p.Item(i).(torrserver.TSListItem)
	b := p.Item(j).(torrserver.TSListItem)
	switch attributeKey {
	case "Size":
		return a.Torrent_Size < b.Torrent_Size
	}
	return false
}

// method overload
func (p *TorrserverList) Reload() {

	result, err := torrserver.List()
	if err != nil {
		utils.LogError(errors.Wrap(err, "Reload"))
	}

	p.Alloc(len(*result))
	for i := range *result {
		p.Append((*result)[i])
	}
	p.Paginator.Reload()
}

// -------------------------------------------------------------------------
func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	var p = NewPaginator()
	p.Sorting.Setup([]paginator.SortHeader{
		{Name: "Size", ShortName: "size", Order: 1},
	})
	p.Reload()
	p.Show(ctx, b, update.Message.Chat.ID)
}
