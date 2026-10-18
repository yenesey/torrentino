// Package presentation derives the plain, UI-independent values shown for
// list rows and download totals. Telegram and web add their own markup around
// these values instead of recalculating percentages, sizes and status peers.
package presentation

import (
	"fmt"
	"math"

	"torrentino/common/utils"
	"torrentino/core"
)

// Item is the presentation view of one list row. It is intentionally a union
// of the fields used by the find, downloads and torrserver screens so both
// frontends can embed the same type.
type Item struct {
	// find
	Title        string
	Size         string
	Tracker      string
	Seeds        uint
	Peers        uint
	HasLink      bool
	HasMagnet    bool
	InTorrents   bool
	InTorrserver bool

	// downloads
	Icon         string
	Name         string
	Status       string
	StatusPeers  uint64
	Percent      string
	PercentValue int
	Ratio        string
	OnDisk       bool
	IsDir        bool
	Ext          string
	ExtCount     int

	// torrserver
	TorrentSize string
}

// NewFindRow derives the display values of a Jackett result.
func NewFindRow(item *core.FindItem) Item {
	if item == nil {
		return Item{}
	}
	return Item{
		Title:        item.Title,
		Size:         utils.FormatFileSize(uint64(item.Size)),
		Tracker:      item.TrackerId,
		Seeds:        item.Seeders,
		Peers:        item.Peers,
		HasLink:      item.Link != "",
		HasMagnet:    item.MagnetUri != "",
		InTorrents:   item.InTorrents,
		InTorrserver: item.InTorrserver,
	}
}

// NewDownloadRow derives the display values of a Transmission or on-disk item.
func NewDownloadRow(item *core.DownloadItem) Item {
	if item == nil {
		return Item{}
	}
	percent := item.PercentDoneValue() * 100
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	percentValue := int(math.Round(percent))

	var peers uint64
	switch item.Status {
	case "seeding":
		if value := item.PeersGettingFromUsValue(); value > 0 {
			peers = uint64(value)
		}
	case "downloading":
		if value := item.PeersSendingToUsValue(); value > 0 {
			peers = uint64(value)
		}
	}

	return Item{
		Icon:         item.Icon(),
		Name:         item.NameValue(),
		Size:         utils.FormatFileSize(uint64(item.DownloadedEverValue())),
		Status:       item.Status,
		StatusPeers:  peers,
		Percent:      fmt.Sprintf("%d%%", percentValue),
		PercentValue: percentValue,
		Ratio:        fmt.Sprintf("%.2fx", item.UploadRatioValue()),
		OnDisk:       item.Status == "unknown",
		IsDir:        item.IsDir,
		Ext:          item.Ext,
		ExtCount:     item.ExtCount,
	}
}

// NewTorrserverRow derives the display values of a Torrserver entry.
func NewTorrserverRow(item *core.TorrserverItem) Item {
	if item == nil {
		return Item{}
	}
	return Item{
		Title:       item.Title,
		TorrentSize: utils.FormatFileSize(uint64(item.Torrent_Size)),
	}
}

// DownloadSummary is the shared, formatted footer data of the downloads
// screen. HasVolume is false when Statfs failed.
type DownloadSummary struct {
	Downloaded string
	Uploaded   string
	Used       string
	Free       string
	HasVolume  bool
}

// NewDownloadSummary formats the downloaded/uploaded totals and, when
// available, the disk volume figures.
func NewDownloadSummary(items []*core.DownloadItem, stats core.DiskStats) DownloadSummary {
	downloaded, uploaded := core.DownloadTotals(items)
	summary := DownloadSummary{
		Downloaded: utils.FormatFileSize(downloaded),
		Uploaded:   utils.FormatFileSize(uploaded),
		HasVolume:  stats.OK,
	}
	if stats.OK {
		summary.Used = utils.FormatFileSize(stats.Used)
		summary.Free = utils.FormatFileSize(stats.Free)
	}
	return summary
}
