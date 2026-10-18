package downloads

import (
	"context"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"torrentino/common/paginator"
	"torrentino/common/utils"
	"torrentino/core"
	"torrentino/core/presentation"
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
	row := presentation.NewDownloadRow(p.Item(i))
	result := ""
	if row.IsDir {
		result = "📁[" + strconv.Itoa(row.ExtCount) + "x | " + row.Ext + "]"
	}

	result += row.Icon + row.Name +
		" [" + row.Size + "]" +
		" [" + row.Percent + "]" +
		" [" + row.Ratio + "]"

	switch row.Status {
	case "seeding", "downloading":
		return result + " [" + row.Status + ":" + strconv.FormatUint(row.StatusPeers, 10) + "p]"
	default:
		return result + " [" + row.Status + "]"
	}
}

// method overload
func (p *ListPaginator) Footer() string {
	items := make([]*core.DownloadItem, 0, p.Len())
	for i := 0; i < p.Len(); i++ {
		items = append(items, p.Item(i))
	}
	summary := presentation.NewDownloadSummary(items, p.stats)
	result := summary.Downloaded + " download / " + summary.Uploaded + " upload"
	if summary.HasVolume {
		result += " \nvolume: " + summary.Used + " used / " + summary.Free + " free"
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
