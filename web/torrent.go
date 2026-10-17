package web

import (
	"errors"
	"log"
	"mime"
	"net/http"

	"torrentino/core"
)

func (s *Server) handleFindTorrent(w http.ResponseWriter, r *http.Request, session *Session) {
	screen := s.screenFor(session, KindFind)

	index, ok := parseIndex(r.URL.Query().Get("item"))
	if !ok {
		s.badRequest(w, r, "invalid item")
		return
	}

	screen.mu.Lock()
	if index >= screen.View.Len() {
		screen.Err = core.ErrStaleItem
		screen.mu.Unlock()
		s.finishCommand(w, r, session, screen, http.StatusConflict)
		return
	}
	item, ok := screen.View.Item(index).(*core.FindItem)
	if !ok {
		screen.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	copied := *item
	screen.mu.Unlock()

	if copied.Link == "" {
		http.NotFound(w, r)
		return
	}

	data, err := s.service.TorrentFile(r.Context(), copied.Link)
	if err != nil {
		log.Printf("[Torrentino]: web: torrent download request_id=%s: %v", requestID(r), err)
		switch {
		case errors.Is(err, core.ErrTorrentTooLarge):
			http.Error(w, "torrent file is too large", http.StatusRequestEntityTooLarge)
		case errors.Is(err, core.ErrInvalidInput), errors.Is(err, core.ErrUpstream):
			http.Error(w, "torrent is unavailable", http.StatusBadGateway)
		default:
			http.Error(w, "torrent is unavailable", http.StatusBadGateway)
		}
		return
	}

	filename := core.SafeTorrentName(copied.Title) + ".torrent"
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	if disposition == "" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", "application/x-bittorrent")
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}
