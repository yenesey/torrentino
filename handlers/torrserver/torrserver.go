package torrserver

import (
	"context"
	"torrentino/common/paginator"
	"torrentino/common/utils"
	"torrentino/api/torrserver"
	"log"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type TorrserverList struct {
	paginator.Paginator
}

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
		log.Fatalf("ItemString")
	}
	return ""
}

// method overload
func (p *TorrserverList) Reload() {

	var result, err = torrserver.ListItems()
	if err != nil {
		log.Fatal(err)
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
