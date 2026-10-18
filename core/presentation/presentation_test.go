package presentation

import (
	"testing"

	"github.com/hekmon/transmissionrpc/v2"

	"torrentino/core"
)

func TestNewDownloadRow(t *testing.T) {
	name := "Movie.mkv"
	downloaded := int64(1024 * 1024)
	percent := 0.9964
	ratio := 1.5
	peers := int64(3)
	row := NewDownloadRow(&core.DownloadItem{
		Torrent: transmissionrpc.Torrent{
			Name:               &name,
			DownloadedEver:     &downloaded,
			PercentDone:        &percent,
			UploadRatio:        &ratio,
			PeersGettingFromUs: &peers,
		},
		Ext:    ".mkv",
		Status: "seeding",
	})

	if row.Name != name || row.Icon != "🎬" {
		t.Errorf("name/icon = %q/%q", row.Name, row.Icon)
	}
	if row.PercentValue != 100 || row.Percent != "100%" {
		t.Errorf("percent = %d/%q, want 100/100%%", row.PercentValue, row.Percent)
	}
	if row.Ratio != "1.50x" {
		t.Errorf("ratio = %q, want 1.50x", row.Ratio)
	}
	if row.StatusPeers != 3 {
		t.Errorf("status peers = %d, want 3", row.StatusPeers)
	}
	if row.Size == "" {
		t.Error("size is empty")
	}
}

func TestNewDownloadRowClampsPercent(t *testing.T) {
	low := -0.5
	high := 1.5
	lowRow := NewDownloadRow(&core.DownloadItem{Torrent: transmissionrpc.Torrent{PercentDone: &low}})
	highRow := NewDownloadRow(&core.DownloadItem{Torrent: transmissionrpc.Torrent{PercentDone: &high}})
	if lowRow.PercentValue != 0 || lowRow.Percent != "0%" {
		t.Errorf("low = %d/%q", lowRow.PercentValue, lowRow.Percent)
	}
	if highRow.PercentValue != 100 || highRow.Percent != "100%" {
		t.Errorf("high = %d/%q", highRow.PercentValue, highRow.Percent)
	}
}

func TestNewDownloadSummary(t *testing.T) {
	downloaded := int64(2 * 1024 * 1024)
	ratio := 2.0
	summary := NewDownloadSummary(
		[]*core.DownloadItem{{Torrent: transmissionrpc.Torrent{
			DownloadedEver: &downloaded,
			UploadRatio:    &ratio,
		}}},
		core.DiskStats{Used: 3 * 1024 * 1024, Free: 4 * 1024 * 1024, OK: true},
	)
	if summary.Downloaded == "" || summary.Uploaded == "" {
		t.Error("download totals are empty")
	}
	if !summary.HasVolume || summary.Used == "" || summary.Free == "" {
		t.Errorf("volume = %+v", summary)
	}
}
