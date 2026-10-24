package web

import (
	"fmt"

	"torrentino/core"
	"torrentino/core/presentation"
)

// NavItem is one header navigation link.
type NavItem struct {
	Kind   Kind
	Label  string
	URL    string
	Active bool
}

// PageData is the data passed to the layout and fragment templates.
type PageData struct {
	Base          string
	CSRF          string
	User          string
	Kind          string
	Nav           []NavItem
	StaticVersion string
	Screen        ScreenView
}

// LoginData is the data passed to the login template.
type LoginData struct {
	Base          string
	Next          string
	CSRF          string
	Error         string
	StaticVersion string
	LoginURL      string
}

type ScreenView struct {
	Kind     string
	Title    string
	Header   string
	Empty    bool
	Rows     []Row
	From     int
	To       int
	Total    int
	Page     int
	LastPage int
	HasPrev  bool
	HasNext  bool

	Controls bool
	Sortings []SortingView
	Filters  []FilterView

	Selected       int
	SelectedNumber int
	SelectedRow    *Row
	Actions        []ActionView

	Footer    string
	Updated   string
	Error     string
	Success   string
	Query     string
	Searching bool

	CommandURL  string
	TorrentBase string
	ReloadURL   string
}

type Row struct {
	presentation.Item

	Index      int
	Number     int
	Selected   bool
	DetailsURL string
	TorrentURL string
}

type SortingView struct {
	Alias  string
	Cmd    string
	Arrow  string
	Queue  int
	Active bool
}

type FilterView struct {
	Attribute string
	Values    []FilterValueView
}

type FilterValueView struct {
	Value   string
	Label   string
	Cmd     string
	Enabled bool
}

type ActionView struct {
	ID     string
	Label  string
	Cmd    string
	Title  string
	Danger bool
}

// BuildPageData wraps a screen view into the layout data.
func (s *Server) buildPageData(session *Session, screenView ScreenView) PageData {
	return PageData{
		Base:          s.base.Prefix(),
		CSRF:          session.CSRF,
		User:          session.User,
		Kind:          screenView.Kind,
		StaticVersion: s.staticVersion,
		Screen:        screenView,
		Nav: []NavItem{
			{Kind: KindFind, Label: "Find", URL: s.base.URL("/find"), Active: screenView.Kind == string(KindFind)},
			{Kind: KindDownloads, Label: "Downloads", URL: s.base.URL("/downloads"), Active: screenView.Kind == string(KindDownloads)},
			{Kind: KindTorrserver, Label: "Torrserver", URL: s.base.URL("/torrserver"), Active: screenView.Kind == string(KindTorrserver)},
		},
	}
}

// buildScreenViewLocked renders a screen into a template-friendly view.
// Callers hold screen.mu.
func (s *Server) buildScreenViewLocked(screen *Screen) ScreenView {
	from, to := screen.View.Bounds()
	total := screen.View.Len()
	view := ScreenView{
		Kind:        string(screen.Kind),
		Title:       screen.Kind.Title(),
		Empty:       total == 0,
		From:        from,
		To:          to,
		Total:       total,
		Page:        screen.View.Page,
		LastPage:    screen.View.LastPage(),
		HasPrev:     screen.View.Page > 0,
		HasNext:     screen.View.Page < screen.View.LastPage(),
		Controls:    screen.View.Controls,
		Selected:    screen.View.Selected,
		Query:       screen.Query,
		Searching:   screen.Searching,
		CommandURL:  s.base.URL("/ui/" + string(screen.Kind) + "/command"),
		TorrentBase: s.base.URL("/find/torrent"),
		Updated:     formatUpdated(screen.Updated),
		Success:     screen.Flash,
	}
	screen.Flash = ""

	switch screen.Kind {
	case KindDownloads:
		view.ReloadURL = s.base.URL("/downloads")
	case KindTorrserver:
		view.ReloadURL = s.base.URL("/torrserver")
	}
	if total == 0 {
		switch {
		case screen.Kind == KindFind && screen.Query != "" && !screen.hasEnabledFiltersLocked():
			view.Header = fmt.Sprintf("No results for “%s”", screen.Query)
		default:
			view.Header = "the list is empty"
		}
	} else {
		view.Header = fmt.Sprintf("results: %d-%d of %d", from+1, to, total)
	}
	if screen.Err != nil {
		view.Error = describeError(screen.Err)
	}

	for _, sorting := range screen.View.Sortings() {
		arrow := ""
		switch sorting.Order {
		case 1:
			arrow = "▼"
		case 2:
			arrow = "▲"
		}
		view.Sortings = append(view.Sortings, SortingView{
			Alias:  sorting.Alias,
			Cmd:    "sort:" + sorting.Attribute,
			Arrow:  arrow,
			Queue:  screen.View.SortingQueuePosition(sorting.Attribute),
			Active: sorting.Order != 0,
		})
	}
	for _, attribute := range screen.View.FilterAttributes() {
		filter := FilterView{Attribute: attribute}
		for _, value := range screen.View.FilterValues(attribute) {
			label := value.Value
			if value.Enabled {
				label = "✓ " + label
			}
			filter.Values = append(filter.Values, FilterValueView{
				Value:   value.Value,
				Label:   label,
				Cmd:     "filter:" + attribute + "/" + value.Value,
				Enabled: value.Enabled,
			})
		}
		view.Filters = append(view.Filters, filter)
	}

	for i := from; i < to; i++ {
		view.Rows = append(view.Rows, s.buildRowLocked(screen, i))
	}

	if screen.View.Selected >= 0 && screen.View.Selected < total {
		row := s.buildRowLocked(screen, screen.View.Selected)
		view.SelectedNumber = screen.View.Selected + 1
		view.SelectedRow = &row
		for _, action := range actionsFor(screen.Kind, s.service, screen.View.Item(screen.View.Selected)) {
			view.Actions = append(view.Actions, ActionView{
				ID:     action.ID,
				Label:  action.Label,
				Cmd:    "action:" + action.ID,
				Title:  actionTitle(screen.Kind, screen.View.Item(screen.View.Selected), action),
				Danger: dangerAction(action),
			})
		}
	}

	if screen.Kind == KindDownloads {
		items := make([]*core.DownloadItem, 0, total)
		for i := 0; i < total; i++ {
			if item, ok := screen.View.Item(i).(*core.DownloadItem); ok {
				items = append(items, item)
			}
		}
		summary := presentation.NewDownloadSummary(items, screen.stats)
		view.Footer = summary.Downloaded + " downloaded / " + summary.Uploaded + " uploaded"
		if summary.HasVolume {
			view.Footer += " · volume: " + summary.Used + " used / " + summary.Free + " free"
		}
	}
	return view
}

// buildRowLocked builds one row. Callers hold screen.mu or own the screen.
func (s *Server) buildRowLocked(screen *Screen, i int) Row {
	row := Row{Index: i, Number: i + 1, Selected: screen.View.Selected == i}
	switch item := screen.View.Item(i).(type) {
	case *core.FindItem:
		row.Item = presentation.NewFindRow(item)
		if core.SafeHTTPURL(item.Details) {
			row.DetailsURL = item.Details
		}
		row.TorrentURL = fmt.Sprintf("%s?item=%d", s.base.URL("/find/torrent"), i)
	case *core.DownloadItem:
		row.Item = presentation.NewDownloadRow(item)
	case *core.TorrserverItem:
		row.Item = presentation.NewTorrserverRow(item)
	}
	return row
}
