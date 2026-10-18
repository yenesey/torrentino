package web

import (
	"errors"
	"sync"
	"time"

	"torrentino/common"
	"torrentino/common/list"
	"torrentino/core"
)

// Kind identifies one of the three screens.
type Kind string

const (
	KindFind       Kind = "find"
	KindDownloads  Kind = "downloads"
	KindTorrserver Kind = "torrserver"
)

func ParseKind(value string) (Kind, bool) {
	switch Kind(value) {
	case KindFind, KindDownloads, KindTorrserver:
		return Kind(value), true
	default:
		return "", false
	}
}

func (k Kind) Title() string {
	switch k {
	case KindFind:
		return "Find"
	case KindDownloads:
		return "Downloads"
	case KindTorrserver:
		return "Torrserver"
	default:
		return string(k)
	}
}

// Screen is the server-side state of one screen in one session.
type Screen struct {
	mu    sync.Mutex // protects the fields below
	opMu  sync.Mutex // serialises commands (including network/disk calls)
	Kind  Kind
	View  list.View
	Query string

	SelectedKey string
	Err         error
	Updated     time.Time
	Searching   bool
	Flash       string
	stats       core.DiskStats
	gen         uint64
}

var (
	errTooManySearches = errors.New("too many searches")
	errUpdateFailed    = errors.New("update failed")
)

func (s *Server) screenFor(session *Session, kind Kind) *Screen {
	return session.GetScreen(kind, func() *Screen {
		return newScreen(kind, common.Settings.WebPageSize())
	})
}

func newScreen(kind Kind, perPage int) *Screen {
	s := &Screen{Kind: kind}
	s.View = list.NewView(perPage)
	s.View.List.Evaluator = kindEvaluator{screen: s}
	s.resetDescriptors()
	return s
}

func (s *Screen) resetDescriptors() {
	switch s.Kind {
	case KindFind:
		s.View.SetupSorting(core.FindSorting())
		s.View.SetupFiltering(core.FindFilters())
	case KindDownloads:
		s.View.SetupSorting(core.DownloadsSorting())
		s.View.SetupFiltering(core.DownloadsFilters())
	case KindTorrserver:
		s.View.SetupSorting(core.TorrserverSorting())
	}
}

// kindEvaluator adapts the shared list.View to the three item types.
type kindEvaluator struct {
	screen *Screen
}

func (e kindEvaluator) Compare(i, j int, attribute string) bool {
	a := e.screen.View.ItemAt(i)
	b := e.screen.View.ItemAt(j)
	switch item := a.(type) {
	case *core.FindItem:
		other, _ := b.(*core.FindItem)
		return core.FindCompare(item, other, attribute)
	case *core.DownloadItem:
		other, _ := b.(*core.DownloadItem)
		return core.DownloadCompare(item, other, attribute)
	case *core.TorrserverItem:
		other, _ := b.(*core.TorrserverItem)
		return core.TorrserverCompare(item, other, attribute)
	}
	return false
}

func (e kindEvaluator) Stringify(i int, attribute string) string {
	switch item := e.screen.View.ItemAt(i).(type) {
	case *core.FindItem:
		switch attribute {
		case "TrackerId":
			return item.TrackerId
		case "TrackerType":
			return item.TrackerType
		}
	case *core.DownloadItem:
		if attribute == "Status" {
			return item.Status
		}
	}
	return ""
}

// screenKey returns the stable identity used to keep the selection on the same
// item across reloads.
func screenKey(kind Kind, item any) string {
	switch kind {
	case KindFind:
		if value, ok := item.(*core.FindItem); ok {
			return core.FindKey(value)
		}
	case KindDownloads:
		if value, ok := item.(*core.DownloadItem); ok {
			return core.DownloadKey(value)
		}
	case KindTorrserver:
		if value, ok := item.(*core.TorrserverItem); ok {
			return core.TorrserverKey(value)
		}
	}
	return ""
}

func actionsFor(kind Kind, service *core.Service, item any) []core.Action {
	switch kind {
	case KindFind:
		if value, ok := item.(*core.FindItem); ok {
			return service.FindActions(value)
		}
	case KindDownloads:
		if value, ok := item.(*core.DownloadItem); ok {
			return service.DownloadActions(value)
		}
	case KindTorrserver:
		if value, ok := item.(*core.TorrserverItem); ok {
			return service.TorrserverActions(value)
		}
	}
	return nil
}

func actionTitle(kind Kind, item any, action core.Action) string {
	switch action.ID {
	case "delete":
		switch kind {
		case KindDownloads:
			if value, ok := item.(*core.DownloadItem); ok && value.Status == "unknown" {
				return "Delete files now — cannot be undone"
			}
			return "Delete now — cannot be undone"
		case KindTorrserver:
			return "Remove from Torrserver; files on disk are not touched"
		}
	}
	return ""
}

func dangerAction(action core.Action) bool {
	return action.ID == "delete"
}

func (s *Screen) hasEnabledFiltersLocked() bool {
	for _, attribute := range s.View.FilterAttributes() {
		for _, value := range s.View.FilterValues(attribute) {
			if value.Enabled {
				return true
			}
		}
	}
	return false
}

// selectedIndexLocked returns the current selection if it is still
// valid. Callers hold s.mu.
func (s *Screen) selectedIndexLocked() (int, bool) {
	if s.View.Selected < 0 || s.View.Selected >= s.View.Len() {
		return -1, false
	}
	return s.View.Selected, true
}

// selectedKeyLocked returns the stable key of the selected item. Callers hold
// s.mu.
func (s *Screen) selectedKeyLocked() string {
	i, ok := s.selectedIndexLocked()
	if !ok {
		return ""
	}
	return screenKey(s.Kind, s.View.Item(i))
}

// restoreSelectionLocked finds the item with the given stable key and selects
// it without closing the controls panel. Callers hold s.mu.
func (s *Screen) restoreSelectionLocked(key string) {
	s.SelectedKey = key
	if key == "" {
		s.View.Selected = -1
		return
	}
	for i := 0; i < s.View.Len(); i++ {
		if screenKey(s.Kind, s.View.Item(i)) == key {
			s.View.Selected = i
			return
		}
	}
	s.View.Selected = -1
	s.SelectedKey = ""
}

// applyFindFlags copies success flags from an executed copy back to the item
// that is still in the list.
func (s *Screen) applyFindFlags(key string, updated *core.FindItem) {
	if updated == nil {
		return
	}
	for i := 0; i < s.View.Len(); i++ {
		item, ok := s.View.Item(i).(*core.FindItem)
		if !ok {
			continue
		}
		if core.FindKey(item) == key {
			item.InTorrents = updated.InTorrents
			item.InTorrserver = updated.InTorrserver
			if updated.InfoHash != "" {
				item.InfoHash = updated.InfoHash
			}
			return
		}
	}
}

// removeLocked removes the selected item by stable key and clears the
// selection. Callers hold s.mu.
func (s *Screen) removeLocked(key string) {
	for i := 0; i < s.View.Len(); i++ {
		if screenKey(s.Kind, s.View.Item(i)) == key {
			s.View.List.Delete(i)
			break
		}
	}
	s.View.Prepare()
	s.View.Selected = -1
	s.SelectedKey = ""
}

// describeError returns a safe message for the banner. Internal details stay
// in the log.
func describeError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, core.ErrStaleItem):
		return "The list changed. Reload and try again."
	case errors.Is(err, core.ErrNotAvailable):
		return "That action is no longer available. Reload the list."
	case errors.Is(err, core.ErrInvalidInput):
		return "Enter a search query between 1 and 256 characters."
	case errors.Is(err, core.ErrUnsafePath):
		return "Refusing to delete: the path is outside the configured download roots."
	case errors.Is(err, core.ErrUpstream):
		return "The service is unavailable. Try again."
	case errors.Is(err, core.ErrTorrentTooLarge):
		return "The torrent file is too large."
	case errors.Is(err, errTooManySearches):
		return "Too many searches. Try again in a minute."
	case errors.Is(err, errUpdateFailed):
		return "Update failed. Showing the previous data."
	default:
		return "The operation failed. Try again."
	}
}

func formatUpdated(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04:05")
}
