package web

import (
	"log"
	"net/http"
	"time"
)

func (s *Server) handleTorrserver(w http.ResponseWriter, r *http.Request, session *Session) {
	screen := s.screenFor(session, KindTorrserver)

	screen.mu.Lock()
	generation := screen.gen
	selectedKey := screen.selectedKeyLocked()
	screen.mu.Unlock()

	items, err := s.service.TorrserverList(r.Context())

	screen.mu.Lock()
	if screen.gen == generation {
		if err == nil {
			screen.View.Alloc(len(items))
			for _, item := range items {
				screen.View.Append(item)
			}
			screen.View.Prepare()
			screen.restoreSelectionLocked(selectedKey)
			screen.Updated = time.Now()
			screen.Err = nil
		} else {
			screen.Err = err
		}
	}
	screen.mu.Unlock()

	if err != nil {
		log.Printf("[Torrentino]: web: torrserver reload failed request_id=%s: %v", requestID(r), err)
	}
	s.renderScreen(w, r, session, screen, http.StatusOK)
}
