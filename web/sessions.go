package web

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"
)

const (
	sessionCookie = "torrentino_session"
	loginCookie   = "torrentino_login"
	maxSessions   = 512
)

// Session is one logged-in browser session. It owns its screens, so different
// users (and different sessions of the same user) never share state.
type Session struct {
	mu       sync.Mutex
	User     string
	CSRF     string
	Created  time.Time
	LastSeen time.Time
	Screens  map[Kind]*Screen
}

func newSession(user string, now time.Time) (*Session, error) {
	csrf, err := randomToken()
	if err != nil {
		return nil, err
	}
	return &Session{
		User:     user,
		CSRF:     csrf,
		Created:  now,
		LastSeen: now,
		Screens:  make(map[Kind]*Screen),
	}, nil
}

// GetScreen returns the session's screen of the given kind, creating it with
// factory on first use.
func (s *Session) GetScreen(kind Kind, factory func() *Screen) *Screen {
	s.mu.Lock()
	defer s.mu.Unlock()
	if screen, ok := s.Screens[kind]; ok {
		return screen
	}
	screen := factory()
	s.Screens[kind] = screen
	return screen
}

type sessionEntry struct {
	token   [32]byte
	session *Session
}

// SessionStore keeps server-side sessions. Only the SHA-256 of the cookie
// token is stored, so a memory dump does not yield usable cookies.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[[32]byte]*sessionEntry
	idle     time.Duration
	absolute time.Duration
	now      func() time.Time
}

func NewSessionStore(idle, absolute time.Duration) *SessionStore {
	return &SessionStore{
		sessions: make(map[[32]byte]*sessionEntry),
		idle:     idle,
		absolute: absolute,
		now:      time.Now,
	}
}

// Create starts a new session and returns the raw cookie token. A fresh token
// is always generated, which protects against session fixation.
func (st *SessionStore) Create(user string) (string, *Session, error) {
	token, err := randomToken()
	if err != nil {
		return "", nil, err
	}
	session, err := newSession(user, st.now())
	if err != nil {
		return "", nil, err
	}
	key := sha256.Sum256([]byte(token))

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.sessions) >= maxSessions {
		st.evictOldestLocked()
	}
	st.sessions[key] = &sessionEntry{token: key, session: session}
	return token, session, nil
}

func (st *SessionStore) evictOldestLocked() {
	var oldestKey [32]byte
	var oldest *Session
	for key, entry := range st.sessions {
		if oldest == nil || entry.session.LastSeen.Before(oldest.LastSeen) {
			oldestKey, oldest = key, entry.session
		}
	}
	if oldest != nil {
		delete(st.sessions, oldestKey)
	}
}

// Get looks up a token and enforces both idle and absolute timeouts. It also
// slides the idle timeout on use.
func (st *SessionStore) Get(token string) (*Session, bool) {
	if token == "" {
		return nil, false
	}
	key := sha256.Sum256([]byte(token))

	st.mu.Lock()
	defer st.mu.Unlock()
	entry, ok := st.sessions[key]
	if !ok {
		return nil, false
	}
	now := st.now()
	session := entry.session
	if now.Sub(session.Created) >= st.absolute || now.Sub(session.LastSeen) >= st.idle {
		delete(st.sessions, key)
		return nil, false
	}
	session.LastSeen = now
	return session, true
}

// Delete invalidates a session (logout).
func (st *SessionStore) Delete(token string) {
	if token == "" {
		return
	}
	key := sha256.Sum256([]byte(token))
	st.mu.Lock()
	delete(st.sessions, key)
	st.mu.Unlock()
}

// GC removes expired sessions. It is cheap enough to run once a minute.
func (st *SessionStore) GC() {
	now := st.now()
	st.mu.Lock()
	defer st.mu.Unlock()
	for key, entry := range st.sessions {
		session := entry.session
		if now.Sub(session.Created) >= st.absolute || now.Sub(session.LastSeen) >= st.idle {
			delete(st.sessions, key)
		}
	}
}

// Len is used by tests and diagnostics.
func (st *SessionStore) Len() int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return len(st.sessions)
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
