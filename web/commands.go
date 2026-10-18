package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"torrentino/core"
)

type webCommand struct {
	verb      string
	index     int
	attribute string
	value     string
	action    string
}

var errBadCommand = errors.New("bad command")

func parseIndex(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, false
		}
	}
	index, err := strconv.Atoi(raw)
	if err != nil || index < 0 {
		return 0, false
	}
	return index, true
}

// parseCommand is the web equivalent of the Telegram dispatcher: a strict
// allowlist with no indexing or Atoi without a checked error.
func parseCommand(raw string) (webCommand, error) {
	switch raw {
	case "deselect":
		return webCommand{verb: "deselect"}, nil
	case "page:next":
		return webCommand{verb: "page", action: "next"}, nil
	case "page:prev":
		return webCommand{verb: "page", action: "prev"}, nil
	case "controls:toggle":
		return webCommand{verb: "controls"}, nil
	}

	if payload, ok := strings.CutPrefix(raw, "select:"); ok {
		index, valid := parseIndex(payload)
		if !valid {
			return webCommand{}, errBadCommand
		}
		return webCommand{verb: "select", index: index}, nil
	}
	if payload, ok := strings.CutPrefix(raw, "sort:"); ok {
		if payload == "" {
			return webCommand{}, errBadCommand
		}
		return webCommand{verb: "sort", attribute: payload}, nil
	}
	if payload, ok := strings.CutPrefix(raw, "filter:"); ok {
		if payload == "clear" {
			return webCommand{verb: "filter-clear"}, nil
		}
		attribute, value, found := strings.Cut(payload, "/")
		if !found || attribute == "" || value == "" {
			return webCommand{}, errBadCommand
		}
		return webCommand{verb: "filter", attribute: attribute, value: value}, nil
	}
	if payload, ok := strings.CutPrefix(raw, "action:"); ok {
		if payload == "" {
			return webCommand{}, errBadCommand
		}
		return webCommand{verb: "action", action: payload}, nil
	}
	return webCommand{}, errBadCommand
}

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request, session *Session) {
	kind, ok := ParseKind(r.PathValue("kind"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	screen := s.screenFor(session, kind)
	// Serialise commands per screen. Network and disk calls happen without
	// screen.mu but still under this operations lock, so a select cannot change
	// the selection in the middle of an action.
	screen.opMu.Lock()
	defer screen.opMu.Unlock()

	cmd, err := parseCommand(r.PostFormValue("cmd"))
	if err != nil {
		s.badRequest(w, r, "invalid command")
		return
	}

	switch cmd.verb {
	case "select":
		s.commandSelect(w, r, session, screen, cmd)
	case "deselect":
		screen.mu.Lock()
		screen.View.Selected = -1
		screen.SelectedKey = ""
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusOK)
	case "page":
		screen.mu.Lock()
		moved := false
		if cmd.action == "next" {
			moved = screen.View.NextPage()
		} else {
			moved = screen.View.PrevPage()
		}
		if moved {
			// The selected index belongs to the old page; drop the stable key
			// too so a later reload/poll does not restore the selection.
			screen.SelectedKey = ""
		}
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusOK)
	case "controls":
		screen.mu.Lock()
		screen.View.ToggleControls()
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusOK)
	case "sort":
		screen.mu.Lock()
		if !screen.View.HasSorting(cmd.attribute) {
			screen.mu.Unlock()
			s.badRequest(w, r, "unknown sort attribute")
			return
		}
		screen.View.ToggleSorting(cmd.attribute)
		screen.View.Prepare()
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusOK)
	case "filter":
		screen.mu.Lock()
		if !slices.Contains(screen.View.FilterAttributes(), cmd.attribute) {
			screen.mu.Unlock()
			s.badRequest(w, r, "unknown filter attribute")
			return
		}
		screen.View.ToggleFilter(cmd.attribute, cmd.value)
		screen.View.Prepare()
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusOK)
	case "filter-clear":
		screen.mu.Lock()
		screen.View.ClearFilters()
		screen.View.Page = 0
		screen.View.Selected = -1
		screen.View.Prepare()
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusOK)
	case "action":
		s.commandAction(w, r, session, screen, cmd.action)
	default:
		s.badRequest(w, r, "invalid command")
	}
}

func (s *Server) commandSelect(w http.ResponseWriter, r *http.Request, session *Session, screen *Screen, cmd webCommand) {
	screen.mu.Lock()
	if screen.View.Selected == cmd.index {
		screen.View.Deselect()
		screen.SelectedKey = ""
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusOK)
		return
	}
	if !screen.View.Select(cmd.index) {
		screen.Err = core.ErrStaleItem
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusConflict)
		return
	}
	key := screen.selectedKeyLocked()
	screen.SelectedKey = key
	screen.Err = nil
	item := screen.View.Item(cmd.index)
	findItem, isFind := item.(*core.FindItem)
	var copied core.FindItem
	if isFind {
		copied = *findItem
	}
	screen.mu.Unlock()

	if isFind && copied.InfoHash == "" && copied.Link != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		err := s.service.ResolveHash(ctx, &copied)
		cancel()

		screen.mu.Lock()
		// The stable key may change when the hash becomes known (it prefers
		// the hash over the link), so verify the selection by identity rather
		// than by the key captured before the lookup.
		if screen.View.Selected == cmd.index && screen.View.ItemAt(cmd.index) == findItem {
			if err != nil {
				log.Printf("[Torrentino]: web: resolve hash request_id=%s: %v", requestID(r), err)
				screen.Err = err
			} else {
				findItem.InfoHash = copied.InfoHash
				findItem.InTorrents = copied.InTorrents
				findItem.InTorrserver = copied.InTorrserver
				screen.Err = nil
				screen.SelectedKey = screen.selectedKeyLocked()
			}
		}
		screen.mu.Unlock()
	}
	s.finishCommand(w, r, session, screen, http.StatusOK)
}

func (s *Server) commandAction(w http.ResponseWriter, r *http.Request, session *Session, screen *Screen, action string) {
	screen.mu.Lock()
	index, ok := screen.selectedIndexLocked()
	if !ok {
		screen.Err = core.ErrStaleItem
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusConflict)
		return
	}
	item := screen.View.Item(index)
	actions := actionsFor(screen.Kind, s.service, item)
	available := false
	for i := range actions {
		if actions[i].ID == action {
			available = true
			break
		}
	}
	if !available {
		screen.Err = core.ErrNotAvailable
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusConflict)
		return
	}

	key := screenKey(screen.Kind, item)
	var (
		findCopy     *core.FindItem
		downloadCopy *core.DownloadItem
		torrentCopy  *core.TorrserverItem
		displayName  string
	)
	switch value := item.(type) {
	case *core.FindItem:
		copied := *value
		findCopy = &copied
		displayName = value.Title
	case *core.DownloadItem:
		copied := *value
		downloadCopy = &copied
		displayName = value.NameValue()
	case *core.TorrserverItem:
		copied := *value
		torrentCopy = &copied
		displayName = value.Title
	}
	screen.gen++
	generation := screen.gen
	screen.mu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	var err error
	switch screen.Kind {
	case KindFind:
		err = s.service.DoFind(ctx, findCopy, action)
	case KindDownloads:
		err = s.service.DoDownload(ctx, downloadCopy, action)
	case KindTorrserver:
		err = s.service.DoTorrserver(ctx, torrentCopy, action)
	}

	screen.mu.Lock()
	if screen.gen != generation {
		// A concurrent search replaced the list while the action ran. The
		// action may have succeeded, but applying its result to the new list
		// would be wrong, so only report the current state.
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusOK)
		return
	}
	if err != nil {
		log.Printf("[Torrentino]: web: action %q failed request_id=%s: %v", action, requestID(r), err)
		screen.Err = err
		screen.mu.Unlock()
		status := http.StatusOK
		if errors.Is(err, core.ErrStaleItem) || errors.Is(err, core.ErrNotAvailable) ||
			errors.Is(err, core.ErrUnsafePath) {
			status = http.StatusConflict
		}
		s.finishCommand(w, r, session, screen, status)
		return
	}

	screen.Err = nil
	screen.View.Selected = -1
	screen.SelectedKey = ""
	switch screen.Kind {
	case KindFind:
		screen.applyFindFlags(key, findCopy)
	case KindDownloads:
		if action == "delete" {
			screen.removeLocked(key)
			s.downloads.Invalidate()
		}
	case KindTorrserver:
		if action == "delete" {
			screen.removeLocked(key)
		}
	}
	screen.Flash = successMessage(action, displayName)
	screen.gen++
	screen.mu.Unlock()

	if action == "delete" {
		log.Printf("[Torrentino]: web: audit delete user=%q kind=%s item=%q key=%q request_id=%s",
			session.User, screen.Kind, displayName, key, requestID(r))
	}
	log.Printf("[Torrentino]: web: action %q ok user=%q request_id=%s", action, session.User, requestID(r))
	s.finishCommand(w, r, session, screen, http.StatusOK)
}

func successMessage(action, name string) string {
	switch action {
	case "delete":
		return fmt.Sprintf("Deleted “%s”.", name)
	case "download", "dwnld:series", "dwnld:movie":
		return "Added to Transmission."
	case "torrsrv":
		return "Added to Torrserver."
	case "pause":
		return "Paused."
	case "start":
		return "Started."
	default:
		return "Done."
	}
}

// finishCommand returns a fragment for htmx and a PRG redirect for a plain
// form submission. Error responses are rendered in place so the message is
// visible without JavaScript.
func (s *Server) finishCommand(w http.ResponseWriter, r *http.Request, session *Session, screen *Screen, status int) {
	if s.htmx(r) || status >= http.StatusBadRequest {
		s.renderScreen(w, r, session, screen, status)
		return
	}
	s.redirect(w, r, s.base.URL("/"+string(screen.Kind)))
}
