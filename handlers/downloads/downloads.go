package downloads

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gensword/collections"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/hekmon/transmissionrpc/v2"

	"torrentino/api/transmission"
	"torrentino/common"
	"torrentino/common/paginator"
	"torrentino/common/utils"
)

var ExtIcons map[string]string = map[string]string{
	".avi":  "🎬",
	".mkv":  "🎬",
	".mp4":  "🎬",
	".m4v":  "🎬",
	".mov":  "🎬",
	".bdmv": "🎬",
	".vob":  "🎬",
	".ts":   "🎬",
	".mp3":  "🎧",
	".wav":  "🎧",
	".m3u":  "🎧",
	".ogg":  "🎧",
	"":      "📄",
}

type ListItem struct {
	transmissionrpc.Torrent
	Ext      string
	ExtCount int
	IsDir    bool
	Status   string
}

type ListPaginator struct {
	paginator.Paginator
}

// ----------------------------------------
func NewPaginator(ctx context.Context, b *bot.Bot, update *models.Update) *ListPaginator {
	var p ListPaginator
	p = ListPaginator{
		*paginator.New(ctx, b, update, "list", 4, &p, &p, &p),
	}
	return &p
}

func (p *ListPaginator) Item(i int) *ListItem {
	return p.Paginator.Item(i).(*ListItem)
}

// method overload
func (p *ListPaginator) Line(i int) string {
	result := ""
	item := p.Item(i)
	if item.IsDir {
		result = "📁[" + strconv.Itoa(item.ExtCount) + "x | " + item.Ext + "]"
	}
	var peersGettingFromUs int64
	var peersSendingToUs int64
	var uploadRatio float64
	if item.PeersSendingToUs == nil {
		item.PeersSendingToUs = &peersSendingToUs
	}

	if item.PeersGettingFromUs == nil {
		item.PeersConnected = &peersGettingFromUs
	}

	if item.UploadRatio == nil || *item.UploadRatio < 0 {
		item.UploadRatio = &uploadRatio
	}

	result = result +
		ExtIcons[item.Ext] +
		"" + *item.Name +
		" [" + utils.FormatFileSize(uint64(*item.DownloadedEver)) + "]" +
		" [" + fmt.Sprintf("%.0f", *item.PercentDone*100) + "%]" +
		" [" + fmt.Sprintf("%.2f", *item.UploadRatio) + "x]" +
		(func() string {
			switch item.Status {
			case "seeding":
				return " [" + item.Status + ":" + fmt.Sprintf("%dp", *item.PeersGettingFromUs) + "]"
			case "downloading":
				return " [" + item.Status + ":" + fmt.Sprintf("%dp", *item.PeersSendingToUs) + "]"
			}
			return " [" + item.Status + "]"
		})()

	return result
}

// method overload
func (p *ListPaginator) Footer() string {

	fs := syscall.Statfs_t{}
	err := syscall.Statfs(common.Settings.Download_dir, &fs)
	if err != nil {
		return ""
	}
	diskAll := fs.Blocks * uint64(fs.Bsize)
	diskFree := fs.Bfree * uint64(fs.Bsize)
	diskUsed := diskAll - diskFree

	var downloaded uint64
	var uploaded uint64
	for i := range p.Len() {
		item := p.Item(i)
		downloaded += uint64(*item.DownloadedEver)
		uploaded += uint64(*item.UploadRatio * float64(*item.DownloadedEver))
	}

	return utils.FormatFileSize(downloaded) + " downoad / " +
		utils.FormatFileSize(uploaded) + " upload " + "\nvolume: " +
		utils.FormatFileSize(diskUsed) + " used / " +
		utils.FormatFileSize(diskFree) + " free"
	// + utils.FormatFileSize(diskAll) + " total "
}

// method overload
func (p *ListPaginator) Stringify(i int, attribute string) string {
	item := p.Item(i)
	if attribute == "Status" {
		return item.Status
	}
	return ""
}

// method overload
func (p *ListPaginator) Compare(i int, j int, attribute string) bool {
	a := p.Item(i)
	b := p.Item(j)

	switch attribute {
	case "AddedDate":
		return (*a.AddedDate).Compare(*b.AddedDate) == -1
	case "Name":
		return *a.Name < *b.Name
	case "DownloadedEver":
		return *a.DownloadedEver < *b.DownloadedEver
	case "IsDir":
		return b.IsDir && !a.IsDir
	}
	return false
}

// method overload
func (p *ListPaginator) Actions(i int) (result []string) {
	item := p.Item(i)

	switch item.Status {
	case "downloading", "seeding":
		result = append(result, "pause")
	default:
		if item.Status != "unknown" {
			result = append(result, "start")
		}
	}
	result = append(result, "delete")
	return result
}

// method overload
func (p *ListPaginator) Execute(i int, action string) (unselect bool) {
	var err error
	item := p.Item(i)
	switch action {
	case "delete":
		if item.ID != nil {
			err = transmission.Delete(*item.ID)
		} else {
			if item.IsDir {
				err = os.RemoveAll(common.Settings.Download_dir + "/" + *item.Name)
			} else {
				err = os.Remove(common.Settings.Download_dir + "/" + *item.Name)
			}
		}
		if err == nil {
			p.Delete(i)
			p.Sort()
		}
	case "start":
		err = transmission.Start(*item.ID)
	case "pause":
		err = transmission.Pause(*item.ID)
	}

	if err != nil {
		utils.LogError(err)
	}
	return true
}

func (p *ListPaginator) Reload() error {

	torrents, err := transmission.List()
	if err != nil {
		utils.LogError(err)
		return err
	}

	listItems := make([]ListItem, len(*torrents), len(*torrents)*2)
	torrentNames := make(map[string]bool)
	for i := range *torrents {
		listItems[i] = ListItem{(*torrents)[i], "", 0, false, ""}

		extCounter := collections.NewCounter()
		for _, file := range (*torrents)[i].Files {
			extCounter.Add(filepath.Ext((*file).Name))
		}
		if extCounter.Len() > 0 {
			mostCommon := extCounter.MostCommon(1)[0]
			listItems[i].Ext = strings.ToLower(mostCommon.Key.(string))
			listItems[i].ExtCount = mostCommon.Value
		}
		listItems[i].IsDir = listItems[i].ExtCount > 1
		listItems[i].Status = (*torrents)[i].Status.String()
		torrentNames[*listItems[i].Name] = true
	}

	dir, err := utils.ReadDir(common.Settings.Download_dir, false)
	if err != nil {
		utils.LogError(err)
	} else {

		for dirEntry := range dir {

			if _, ok := torrentNames[dirEntry.Name]; !ok {
				name := dirEntry.Name
				size := int64(dirEntry.Size)
				status := transmissionrpc.TorrentStatus(404)
				modTime := dirEntry.ModTime
				zero := float64(0.0)
				ext := strings.ToLower(filepath.Ext(dirEntry.Name))
				extCount := 0

				extCounter := collections.NewCounter()
				if dirEntry.IsDir {
					if subDirsWalk, err := utils.ReadDir(path.Join(common.Settings.Download_dir, name), true); err == nil {
						for subs := range subDirsWalk {
							extCounter.Add(filepath.Ext(subs.Name))
							size += subs.Size
						}
					} else {
						utils.LogError(err)
					}
				}
				if extCounter.Len() > 0 {
					mostCommon := extCounter.MostCommon(1)[0]
					ext = strings.ToLower(mostCommon.Key.(string))
					extCount = mostCommon.Value
				}

				listItems = append(listItems,
					ListItem{
						transmissionrpc.Torrent{
							Name:           &name,
							DownloadedEver: &size,
							Status:         &status,
							PercentDone:    &zero,
							UploadRatio:    &zero,
							AddedDate:      &modTime,
						},
						ext,
						extCount,
						dirEntry.IsDir,
						"unknown",
					})
			}
		}
	}

	p.Alloc(len(listItems))
	for i := range listItems {
		p.Append(&listItems[i])
	}
	return nil
}

// -------------------------------------------------------------------------
var Updater = func() func(ctx context.Context, p *ListPaginator) {
	var (
		cancel     context.CancelFunc
		updaterCtx context.Context
	)
	return func(ctx context.Context, p *ListPaginator) {
		if cancel != nil {
			cancel()
		}
		updaterCtx, cancel = context.WithCancel(ctx)
		ticker := time.NewTicker(time.Second * 5)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				p.Reload()
				p.Show()
			case <-updaterCtx.Done():
				return
			}
		}
	}
}()

func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {

	p := NewPaginator(ctx, b, update)
	p.SetupSorting([]paginator.Sorting{
		{Attribute: "AddedDate", Alias: "date", Order: 1},
		{Attribute: "Name", Alias: "name", Order: 1},
		{Attribute: "DownloadedEver", Alias: "size", Order: 0},
		{Attribute: "IsDir", Alias: "dir", Order: 0},
	})
	p.SetupFiltering([]string{"Status"})

	if err := p.Reload(); err != nil {
		p.ReplyMessage(err.Error())
	} else {
		p.Show()
		go Updater(ctx, p)
	}
}
