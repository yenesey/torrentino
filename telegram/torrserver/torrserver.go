package torrserver

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"torrentino/common/paginator"
	"torrentino/common/utils"
	"torrentino/core"
	"torrentino/core/presentation"
)

type TorrserverPaginator struct {
	paginator.Paginator
	ctx     context.Context
	service *core.Service
}

// ----------------------------------------
func NewPaginator(ctx context.Context, b *bot.Bot, update *models.Update) *TorrserverPaginator {
	var p TorrserverPaginator
	p.Paginator = *paginator.New(ctx, b, update, "torrserver", 4, &p, &p, &p)
	p.ctx = ctx
	p.service = core.Default
	return &p
}

func (p *TorrserverPaginator) Item(i int) *core.TorrserverItem {
	return p.Paginator.Item(i).(*core.TorrserverItem)
}

// method overload
func (p *TorrserverPaginator) Line(i int) string {
	row := presentation.NewTorrserverRow(p.Item(i))
	return row.Title + " [" + row.TorrentSize + "]"
}

// method overload
func (p *TorrserverPaginator) Actions(i int) []string {
	return core.ActionIDs(p.service.TorrserverActions(p.Item(i)))
}

// method overload
func (p *TorrserverPaginator) Execute(i int, action string) (unselect bool) {
	if err := p.service.DoTorrserver(p.ctx, p.Item(i), action); err != nil {
		utils.LogError(err)
		return true
	}
	p.Delete(i)
	return true
}

// method overload
func (p *TorrserverPaginator) Stringify(i int, attribute string) string {
	return ""
}

// method overload
func (p *TorrserverPaginator) Compare(i int, j int, attribute string) bool {
	return core.TorrserverCompare(p.Item(i), p.Item(j), attribute)
}

func (p *TorrserverPaginator) Reload() error {
	items, err := p.service.TorrserverList(p.ctx)
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

// -------------------------------------------------------------------------
func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	var p = NewPaginator(ctx, b, update)
	p.SetupSorting(core.TorrserverSorting())
	if err := p.Reload(); err != nil {
		p.ReplyMessage(err.Error())
	} else {
		p.Show()
	}
}
