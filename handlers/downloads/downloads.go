package downloads

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"torrentino/common/paginator"
	"torrentino/common/utils"
	"torrentino/core"
)

type ListPaginator struct {
	paginator.Paginator
	ctx     context.Context
	service *core.Service
	stats   core.DiskStats
}

// ----------------------------------------
func NewPaginator(ctx context.Context, b *bot.Bot, update *models.Update) *ListPaginator {
	var p ListPaginator
	p.Paginator = *paginator.New(ctx, b, update, "list", 4, &p, &p, &p)
	p.ctx = ctx
	p.service = core.Default
	return &p
}

func (p *ListPaginator) Item(i int) *core.DownloadItem {
	return p.Paginator.Item(i).(*core.DownloadItem)
}

// method overload
func (p *ListPaginator) Line(i int) string {
	result := ""
	item := p.Item(i)
	if item.IsDir {
		result = "📁[" + strconv.Itoa(item.ExtCount) + "x | " + item.Ext + "]"
	}

	result = result +
		item.Icon() +
		item.NameValue() +
		" [" + utils.FormatFileSize(uint64(item.DownloadedEverValue())) + "]" +
		" [" + fmt.Sprintf("%.0f", item.PercentDoneValue()*100) + "%]" +
		" [" + fmt.Sprintf("%.2f", item.UploadRatioValue()) + "x]" +
		(func() string {
			switch item.Status {
			case "seeding":
				return " [" + item.Status + ":" + fmt.Sprintf("%dp", item.PeersGettingFromUsValue()) + "]"
			case "downloading":
				return " [" + item.Status + ":" + fmt.Sprintf("%dp", item.PeersSendingToUsValue()) + "]"
			}
			return " [" + item.Status + "]"
		})()

	return result
}

// method overload
func (p *ListPaginator) Footer() string {
	items := make([]*core.DownloadItem, 0, p.Len())
	for i := 0; i < p.Len(); i++ {
		items = append(items, p.Item(i))
	}
	downloaded, uploaded := core.DownloadTotals(items)

	result := utils.FormatFileSize(downloaded) + " download / " +
		utils.FormatFileSize(uploaded) + " upload"
	if p.stats.OK {
		result += " " + "\nvolume: " +
			utils.FormatFileSize(p.stats.Used) + " used / " +
			utils.FormatFileSize(p.stats.Free) + " free"
	}
	return result
}

// method overload
func (p *ListPaginator) Stringify(i int, attribute string) string {
	if attribute == "Status" {
		return p.Item(i).Status
	}
	return ""
}

// method overload
func (p *ListPaginator) Compare(i int, j int, attribute string) bool {
	return core.DownloadCompare(p.Item(i), p.Item(j), attribute)
}

// method overload
func (p *ListPaginator) Actions(i int) (result []string) {
	return core.ActionIDs(p.service.DownloadActions(p.Item(i)))
}

// method overload
func (p *ListPaginator) Execute(i int, action string) (unselect bool) {
	if err := p.service.DoDownload(p.ctx, p.Item(i), action); err != nil {
		utils.LogError(err)
		return true
	}
	if action == "delete" {
		p.Delete(i)
		p.Sort()
	}
	return true
}

func (p *ListPaginator) Reload() error {
	items, stats, err := p.service.Downloads(p.ctx)
	if err != nil {
		utils.LogError(err)
		return err
	}
	p.stats = stats

	p.Alloc(len(items))
	for _, item := range items {
		p.Append(item)
	}
	return nil
}

// -------------------------------------------------------------------------
var Updater = func() func(ctx context.Context, p *ListPaginator) {
	var cancel context.CancelFunc
	return func(ctx context.Context, p *ListPaginator) {
		if cancel != nil {
			cancel()
		}
		var updaterCtx context.Context
		updaterCtx, cancel = context.WithCancel(ctx)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := p.Reload(); err != nil {
					utils.LogError(err)
				}
				p.Show()
			case <-updaterCtx.Done():
				return
			}
		}
	}
}()

func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {
	p := NewPaginator(ctx, b, update)
	p.SetupSorting(core.DownloadsSorting())
	p.SetupFiltering(core.DownloadsFilters())

	if err := p.Reload(); err != nil {
		p.ReplyMessage(err.Error())
	} else {
		p.Show()
		go Updater(ctx, p)
	}
}
