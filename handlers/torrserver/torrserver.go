package torrserver

import (
	"context"
	"torrentino/common/paginator"
	"torrentino/common/utils"
	"torrentino/api/torrserver"
	"log"
	"fmt"
	"github.com/pkg/errors"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type TorrserverList struct {
	paginator.Paginator
}
// ----------------------------------------
func logError(err error) {
	log.Printf("[handlers/torrserver] %s", err)
}
// ----------------------------------------
func NewPaginator() *TorrserverList {
	var p TorrserverList
	p = TorrserverList{
		*paginator.New(&p, "torrerver", 4),
	}
	p.Reload()
	return &p
}

// method overload
func (p *TorrserverList) ItemString(item_ any) string {

	if item, ok := item_.(torrserver.TSListItem); ok {
		return item.Title +
		" [" + utils.FormatFileSize(int64(item.Torrent_Size), 1024.0) + "]"
		

	} else {
		logError(fmt.Errorf("ItemString %s", "error"))
	}
	return ""
}

// method overload
func (p *TorrserverList) ItemActions(i int) (result []string) {
	result = []string{"delete"}
	return
}

// method overload
func (p *TorrserverList) ItemActionExec(i int, actionKey string) bool {
	item := p.Item(i).(torrserver.TSListItem)
	if actionKey == "delete" {
		if err := torrserver.Delete(item.Hash); err == nil {
			p.Delete(i)
			p.Refresh()
		} else {
			logError(errors.Wrap(err, "ItemActionExec"))
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
		return a.Torrent_Size  < b.Torrent_Size
	}
	return false
}


// method overload
func (p *TorrserverList) Reload() {

	var result, err = torrserver.List()
	if err != nil {
		logError(errors.Wrap(err, "Reload"))
	}
	
	p.Alloc(len(*result))
	for i := range *result {
		p.Append((*result)[i])
	}

}


//-------------------------------------------------------------------------
func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	var pg = NewPaginator()
	pg.Sorting.Setup([]paginator.SortHeader{
		{Name: "Size", ShortName: "size", Order: 1},
	})
	pg.Show(ctx, b, update.Message.Chat.ID)
}
