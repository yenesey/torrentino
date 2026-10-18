package core

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gensword/collections"
	"github.com/hekmon/transmissionrpc/v2"

	"torrentino/common"
	"torrentino/common/utils"
)

// ExtIcons maps a file extension to the icon shown in both interfaces.
var ExtIcons = map[string]string{
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

// ExtIcon returns the icon for a file extension, falling back to a document
// icon for unknown extensions.
func ExtIcon(ext string) string {
	if icon, ok := ExtIcons[strings.ToLower(ext)]; ok {
		return icon
	}
	return "📄"
}

// DownloadItem is a Transmission torrent or a file that exists on disk only.
// The embedded Torrent fields are pointers and may be nil, so rendering code
// must use the *Value helpers instead of dereferencing them.
type DownloadItem struct {
	transmissionrpc.Torrent
	Ext      string
	ExtCount int
	IsDir    bool
	Status   string // seeding/downloading/paused/idle/unknown
}

func (i *DownloadItem) NameValue() string {
	if i.Name == nil {
		return ""
	}
	return *i.Name
}

func (i *DownloadItem) DownloadedEverValue() int64 {
	if i.DownloadedEver == nil {
		return 0
	}
	return *i.DownloadedEver
}

func (i *DownloadItem) PercentDoneValue() float64 {
	if i.PercentDone == nil {
		return 0
	}
	return *i.PercentDone
}

func (i *DownloadItem) UploadRatioValue() float64 {
	if i.UploadRatio == nil || *i.UploadRatio < 0 {
		return 0
	}
	return *i.UploadRatio
}

func (i *DownloadItem) PeersGettingFromUsValue() int64 {
	if i.PeersGettingFromUs == nil {
		return 0
	}
	return *i.PeersGettingFromUs
}

func (i *DownloadItem) PeersSendingToUsValue() int64 {
	if i.PeersSendingToUs == nil {
		return 0
	}
	return *i.PeersSendingToUs
}

func (i *DownloadItem) AddedDateValue() time.Time {
	if i.AddedDate == nil {
		return time.Time{}
	}
	return *i.AddedDate
}

func (i *DownloadItem) DownloadDirValue() string {
	if i.DownloadDir == nil {
		return ""
	}
	return *i.DownloadDir
}

func (i *DownloadItem) IDValue() (int64, bool) {
	if i.ID == nil {
		return 0, false
	}
	return *i.ID, true
}

// DiskStats is the /downloads footer. OK is false when Statfs failed; the
// downloaded/uploaded totals are still usable in that case.
type DiskStats struct {
	Used  uint64
	Free  uint64
	Total uint64
	OK    bool
}

// Downloads returns Transmission torrents plus files found in the three
// configured download roots. A Transmission failure is returned; an unreadable
// directory is logged and skipped.
func (s *Service) Downloads(ctx context.Context) ([]*DownloadItem, DiskStats, error) {
	ctx, cancel := context.WithTimeout(ctx, serviceTimeout)
	defer cancel()
	stats := diskStats(common.Settings.Path.Default)

	torrents, err := s.Transmission.List(ctx)
	if err != nil {
		return nil, stats, Upstream(err)
	}

	items := make([]*DownloadItem, 0, len(torrents)*2)
	torrentNames := make(map[string]bool)
	for i := range torrents {
		status := "unknown"
		if torrents[i].Status != nil {
			status = torrents[i].Status.String()
		}
		item := &DownloadItem{Torrent: torrents[i], Status: status}
		extCounter := collections.NewCounter()
		for _, file := range torrents[i].Files {
			if file != nil {
				extCounter.Add(filepath.Ext(file.Name))
			}
		}
		if extCounter.Len() > 0 {
			mostCommon := extCounter.MostCommon(1)[0]
			item.Ext = strings.ToLower(mostCommon.Key.(string))
			item.ExtCount = mostCommon.Value
		}
		item.IsDir = item.ExtCount > 1
		torrentNames[item.NameValue()] = true
		items = append(items, item)
	}

	readDir := func(targetDir string) {
		if strings.TrimSpace(targetDir) == "" {
			return
		}
		dir, err := utils.ReadDir(targetDir, false)
		if err != nil {
			utils.LogError(err)
			return
		}
		for dirEntry := range dir {
			if torrentNames[dirEntry.Name] {
				continue
			}
			name := dirEntry.Name
			size := dirEntry.Size
			status := transmissionrpc.TorrentStatus(404)
			modTime := dirEntry.ModTime
			zero := 0.0
			ext := strings.ToLower(filepath.Ext(dirEntry.Name))
			extCount := 0
			extCounter := collections.NewCounter()
			if dirEntry.IsDir {
				if subDirsWalk, err := utils.ReadDir(path.Join(targetDir, name), true); err == nil {
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
			items = append(items, &DownloadItem{
				Torrent: transmissionrpc.Torrent{
					Name:           &name,
					DownloadedEver: &size,
					Status:         &status,
					PercentDone:    &zero,
					UploadRatio:    &zero,
					AddedDate:      &modTime,
					DownloadDir:    &targetDir,
				},
				Ext:      ext,
				ExtCount: extCount,
				IsDir:    dirEntry.IsDir,
				Status:   "unknown",
			})
		}
	}
	readDir(common.Settings.Path.Default)
	readDir(common.Settings.Path.Movie)
	readDir(common.Settings.Path.Series)

	return items, stats, nil
}

func diskStats(root string) DiskStats {
	if strings.TrimSpace(root) == "" {
		return DiskStats{}
	}
	fs := syscall.Statfs_t{}
	if err := syscall.Statfs(root, &fs); err != nil {
		return DiskStats{}
	}
	total := fs.Blocks * uint64(fs.Bsize)
	free := fs.Bfree * uint64(fs.Bsize)
	return DiskStats{Used: total - free, Free: free, Total: total, OK: true}
}

// DownloadTotals sums the downloaded and uploaded bytes across all items.
func DownloadTotals(items []*DownloadItem) (downloaded, uploaded uint64) {
	for _, item := range items {
		downloaded += uint64(item.DownloadedEverValue())
		uploaded += uint64(item.UploadRatioValue() * float64(item.DownloadedEverValue()))
	}
	return downloaded, uploaded
}

// DownloadCompare implements the sorting rules for the downloads screen.
func DownloadCompare(a, b *DownloadItem, attribute string) bool {
	switch attribute {
	case "AddedDate":
		return a.AddedDateValue().Compare(b.AddedDateValue()) == -1
	case "Name":
		return a.NameValue() < b.NameValue()
	case "DownloadedEver":
		return a.DownloadedEverValue() < b.DownloadedEverValue()
	case "IsDir":
		return b.IsDir && !a.IsDir
	}
	return false
}

// DownloadActions returns the actions offered for a download item.
func (s *Service) DownloadActions(item *DownloadItem) []Action {
	var result []Action
	if item == nil {
		return result
	}
	switch item.Status {
	case "downloading", "seeding":
		result = append(result, Action{ID: "pause", Label: "pause"})
	default:
		if item.Status != "unknown" {
			result = append(result, Action{ID: "start", Label: "start"})
		}
	}
	result = append(result, Action{ID: "delete", Label: "delete"})
	return result
}

// DownloadKey is the stable identity of an item across polling reloads:
// the Transmission ID when there is one, the cleaned disk path otherwise.
func DownloadKey(item *DownloadItem) string {
	if item == nil {
		return ""
	}
	if id, ok := item.IDValue(); ok {
		return fmt.Sprintf("id:%d", id)
	}
	return "path:" + filepath.Clean(filepath.Join(item.DownloadDirValue(), item.NameValue()))
}

// DownloadPath returns the on-disk path of an item-only-on-disk after checking
// that it is inside one of path.default/movie/series. It never trusts a path
// from the client: both components come from the server-side list.
func (s *Service) DownloadPath(item *DownloadItem) (string, error) {
	if item == nil || item.NameValue() == "" || item.DownloadDirValue() == "" {
		return "", fmt.Errorf("%w: item has no name or download directory", ErrUnsafePath)
	}
	target := filepath.Clean(filepath.Join(item.DownloadDirValue(), item.NameValue()))
	if !filepath.IsAbs(target) {
		return "", fmt.Errorf("%w: %q is not absolute", ErrUnsafePath, target)
	}

	for _, root := range []string{
		common.Settings.Path.Default,
		common.Settings.Path.Movie,
		common.Settings.Path.Series,
	} {
		if strings.TrimSpace(root) == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absRoot, target)
		if err != nil {
			continue
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return target, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnsafePath, target)
}

// DoDownload executes pause/start/delete for a download item. Deleting an
// item that exists only on disk removes files immediately; the path is checked
// against the configured roots first.
func (s *Service) DoDownload(ctx context.Context, item *DownloadItem, action string) error {
	if item == nil {
		return ErrStaleItem
	}
	timeout := serviceTimeout
	if action == "delete" {
		timeout = deleteTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	switch action {
	case "delete":
		if id, ok := item.IDValue(); ok {
			if err := s.Transmission.Delete(ctx, id); err != nil {
				return Upstream(err)
			}
			return nil
		}
		target, err := s.DownloadPath(item)
		if err != nil {
			return err
		}
		if item.IsDir {
			err = os.RemoveAll(target)
		} else {
			err = os.Remove(target)
		}
		if err != nil {
			return fmt.Errorf("remove %q: %w", target, err)
		}
		return nil
	case "start", "pause":
		id, ok := item.IDValue()
		if !ok {
			return fmt.Errorf("%w: %s needs a Transmission item", ErrNotAvailable, action)
		}
		var err error
		if action == "start" {
			err = s.Transmission.Start(ctx, id)
		} else {
			err = s.Transmission.Pause(ctx, id)
		}
		if err != nil {
			return Upstream(err)
		}
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrNotAvailable, action)
	}
}

// DownloadExtra holds the UI-visible parts derived from the file extension.
func (i *DownloadItem) Icon() string {
	return ExtIcon(i.Ext)
}
