package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"torrentino/core"
)

func (s *Server) handleDownloads(w http.ResponseWriter, r *http.Request, session *Session) {
	screen := s.screenFor(session, KindDownloads)

	screen.mu.Lock()
	generation := screen.gen
	screen.mu.Unlock()

	items, stats, updated, err := s.downloads.Get(r.Context(), s.service)
	if errors.Is(err, context.Canceled) {
		return
	}

	screen.mu.Lock()
	if screen.gen == generation {
		if err == nil {
			s.applyDownloadItemsLocked(screen, items, stats, updated)
			screen.Err = nil
		} else {
			screen.Err = errUpdateFailed
		}
	}
	screen.mu.Unlock()

	if err != nil {
		log.Printf("[Torrentino]: web: downloads reload failed request_id=%s: %v", requestID(r), err)
	}
	s.renderScreen(w, r, session, screen, http.StatusOK)
}

// applyDownloadItemsLocked updates the shared view while keeping sorting,
// filters, page and the selection-by-key. The filter option list is rebuilt
// from the actual statuses, so values that disappeared do not linger.
func (s *Server) applyDownloadItemsLocked(screen *Screen, items []*core.DownloadItem, stats core.DiskStats, updated time.Time) {
	selectedKey := screen.selectedKeyLocked()
	enabled := screen.View.EnabledFilters()

	screen.View.SetupFiltering(core.DownloadsFilters())
	screen.View.Alloc(len(items))
	for _, item := range items {
		screen.View.Append(item)
	}
	for attribute, values := range enabled {
		for value := range values {
			screen.View.SetFilter(attribute, value, true)
		}
	}
	screen.View.Prepare()
	screen.restoreSelectionLocked(selectedKey)
	screen.stats = stats
	if !updated.IsZero() {
		screen.Updated = updated
	}
}
