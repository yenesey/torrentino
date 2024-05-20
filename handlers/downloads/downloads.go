package downloads

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"torrentino/api/transmission"
	"torrentino/common"
	"torrentino/common/paginator"
	"torrentino/common/utils"

	"github.com/hekmon/transmissionrpc/v2"

	"github.com/gensword/collections"

	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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
func logError(err error) {
	log.Printf("[handlers/dowloads] %s", err)
}

// ----------------------------------------
func NewPaginator() *ListPaginator {
	var p ListPaginator
	p = ListPaginator{
		*paginator.New(&p, "list", 4),
	}
	return &p
}

// method overload
func (p *ListPaginator) ItemString(item any) string {
	result := ""
	if data, ok := item.(ListItem); ok {
		if data.IsDir {
			result = "📁[" + strconv.Itoa(data.ExtCount) + "x | " + data.Ext + "]"
		}

		result = result +
			ExtIcons[data.Ext] +
			"" + *data.Name +
			" [" + utils.FormatFileSize(*data.DownloadedEver, 1024) + "]" +
			" [" + fmt.Sprintf("%.2f", *data.PercentDone*100) + "%]" +
			" [" + fmt.Sprintf("%.2f", *data.UploadRatio) + "x]" +
			" [" + data.Status + "]"
	} else {
		logError(fmt.Errorf("ItemString - error get item data"))
	}
	return result
}

// method overload
func (p *ListPaginator) KeepItem(item any, attributeKey string, attributeValue string) bool {
	testItem := item.(ListItem)
	if attributeKey == "Status" {
		if testItem.Status == attributeValue {
			return true
		}
	}
	return false
}

// method overload
func (p *ListPaginator) LessItem(i int, j int, attributeKey string) bool {
	a := p.Item(i).(ListItem)
	b := p.Item(j).(ListItem)
	switch attributeKey {
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
func (p *ListPaginator) ItemActions(i int) (result []string) {
	item := p.Item(i).(ListItem)

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
func (p *ListPaginator) ItemActionExec(i int, actionKey string) bool {
	item := p.Item(i).(ListItem)
	switch actionKey {
	case "delete":
		if item.ID != nil {
			transmission.Delete(*item.ID)
		} else {
			if item.IsDir {
				os.RemoveAll(common.Settings.Download_dir + "/" + *item.Name)
			} else {
				os.Remove(common.Settings.Download_dir + "/" + *item.Name)
			}
		}
		p.Delete(i)
		p.Refresh()

	case "start":
		if transmission.Start(*item.ID) == nil {
			p.Reload()
			p.Refresh()
		} else {
			logError(fmt.Errorf("transmission.Start"))
		}
	case "pause":
		if transmission.Pause(*item.ID) == nil {
			p.Reload()
			p.Refresh()
		} else {
			logError(fmt.Errorf("transmission.Pause"))
		}
	}
	return true
}

// method overload
func (p *ListPaginator) Reload() {

	torrents, err := transmission.List()
	if err != nil {
		log.Fatal(err)
	}

	listItems := make([]ListItem, len(torrents), len(torrents)*2)
	torrentNames := make(map[string]bool)
	for i := range torrents {
		listItems[i] = ListItem{torrents[i], "", 0, false, ""}

		extCounter := collections.NewCounter()
		for _, file := range torrents[i].Files {
			extCounter.Add(filepath.Ext((*file).Name))
		}
		listItems[i].Ext = strings.ToLower(extCounter.MostCommon(1)[0].Key.(string))
		listItems[i].ExtCount = extCounter.MostCommon(1)[0].Value

		listItems[i].IsDir = listItems[i].ExtCount > 1
		listItems[i].Status = torrents[i].Status.String()
		torrentNames[*listItems[i].Name] = true
	}

	dir, err := utils.ReadDir(common.Settings.Download_dir, false)
	if err != nil {
		logError(err)
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
						logError(err)
					}
				}
				if len(extCounter.MostCommon(1)) > 0 {
					ext = strings.ToLower(extCounter.MostCommon(1)[0].Key.(string))
					extCount = extCounter.MostCommon(1)[0].Value
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
		p.Append(listItems[i])
	}
	p.Paginator.Reload()
}

// -------------------------------------------------------------------------
var gDone chan bool = make(chan bool, 1)
var gFirstFun bool = true

func Handler(ctx context.Context, b *bot.Bot, update *models.Update) {

	var p = NewPaginator()
	p.Sorting.Setup([]paginator.SortHeader{
		{Name: "AddedDate", ShortName: "date", Order: 1},
		{Name: "Name", ShortName: "name", Order: 1},
		{Name: "TotalSize", ShortName: "size", Order: 0},
		{Name: "IsDir", ShortName: "dir", Order: 0},
	})
	p.Filtering.Setup([]string{"Status"})

	if !gFirstFun {
		gDone <- true
	}
	gFirstFun = false
	ticker := time.NewTicker(time.Second * 2)
	go func() {
		for {
			select {
			case <- ticker.C:
				p.Reload()
				p.Refresh()
			case <-gDone:
				ticker.Stop()
				return
			}
		}
	}()

	p.Reload()
	p.Show(ctx, b, update.Message.Chat.ID)
}
