package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"torrentino/common"
	"torrentino/common/list"
	"torrentino/core"
)

func (s *Server) handleFind(w http.ResponseWriter, r *http.Request, session *Session) {
	screen := s.screenFor(session, KindFind)
	s.renderScreen(w, r, session, screen, http.StatusOK)
}

func (s *Server) handleFindSearch(w http.ResponseWriter, r *http.Request, session *Session) {
	screen := s.screenFor(session, KindFind)
	if retry, ok := s.searchLimiter.Allow("search:" + s.clientIP(r)); !ok {
		seconds := int(retry.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		screen.mu.Lock()
		screen.Err = errTooManySearches
		screen.mu.Unlock()
		log.Printf("[Torrentino]: web: search rate limited ip=%s request_id=%s", s.clientIP(r), requestID(r))
		s.renderScreen(w, r, session, screen, http.StatusTooManyRequests)
		return
	}
	query := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, r.PostFormValue("q"))
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > 256 {
		screen.mu.Lock()
		screen.Err = core.ErrInvalidInput
		screen.mu.Unlock()
		s.renderScreen(w, r, session, screen, http.StatusBadRequest)
		return
	}

	screen.mu.Lock()
	screen.gen++
	generation := screen.gen
	screen.Searching = true
	screen.Err = nil
	screen.mu.Unlock()

	items, err := s.service.Search(r.Context(), query)
	if errors.Is(err, context.Canceled) {
		// The browser left the page; clear the progress flag if this is still
		// the current search, then let the request die.
		screen.mu.Lock()
		if screen.gen == generation {
			screen.Searching = false
		}
		screen.mu.Unlock()
		return
	}

	screen.mu.Lock()
	if screen.gen != generation {
		// A newer search won; show its progress instead of leaving this HTTP
		// request with an empty body.
		screen.mu.Unlock()
		s.renderScreen(w, r, session, screen, http.StatusOK)
		return
	}
	screen.Searching = false
	if err != nil {
		screen.Err = err
		screen.mu.Unlock()
		log.Printf("[Torrentino]: web: search failed request_id=%s: %v", requestID(r), err)
		if s.htmx(r) {
			s.renderScreen(w, r, session, screen, http.StatusOK)
			return
		}
		s.redirect(w, r, s.base.URL("/find"))
		return
	}

	view := list.NewView(common.Settings.WebPageSize())
	view.List.Evaluator = kindEvaluator{screen: screen}
	screen.View = view
	screen.resetDescriptors()
	screen.Query = query
	screen.SelectedKey = ""
	screen.View.Controls = false
	screen.View.Alloc(len(items))
	for _, item := range items {
		screen.View.Append(item)
	}
	screen.View.Prepare()
	screen.Updated = time.Now()
	screen.Flash = ""
	screen.mu.Unlock()

	if s.htmx(r) {
		w.Header().Set("HX-Push-Url", s.base.URL("/find"))
		s.renderScreen(w, r, session, screen, http.StatusOK)
		return
	}
	s.redirect(w, r, s.base.URL("/find"))
}
