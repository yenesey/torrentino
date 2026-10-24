package web

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"torrentino/common"
	"torrentino/common/password"
)

// dummyPasswordHash is a valid PBKDF2 hash used to equalise the time spent on
// unknown usernames. It protects no secret; the password is a fixed string.
const dummyPasswordHash = "pbkdf2-sha256$600000$3UKoZwbQuBGnaBiTfrIjyg==$0f2L8tFoQiJd2TUCwU+z0P4FettMHLlSYgEDHBpD/5U="

const (
	loginCookieMaxAge = 10 * time.Minute
	loginMaxFailures  = 5
	loginWindow       = 15 * time.Minute
)

func (s *Server) findUser(name string) (common.WebUser, bool) {
	user, ok := s.users[strings.ToLower(strings.TrimSpace(name))]
	return user, ok
}

// verifyLogin checks a username/password pair. Unknown users still pay for one
// PBKDF2 computation against a dummy hash, so timing does not reveal whether
// the account exists.
func (s *Server) verifyLogin(name, pw string) (string, bool) {
	user, ok := s.findUser(name)
	if !ok {
		password.Verify(pw, s.dummyHash)
		return "", false
	}
	if !password.Verify(pw, user.PasswordHash) {
		return "", false
	}
	return user.Name, true
}

func (s *Server) loginKeys(r *http.Request, user string) [2]string {
	ip := s.clientIP(r)
	return [2]string{
		"ip:" + ip,
		"user:" + ip + "|" + strings.ToLower(strings.TrimSpace(user)),
	}
}

func (s *Server) handleLoginGET(w http.ResponseWriter, r *http.Request) {
	if session := s.sessionFromRequest(r); session != nil {
		s.redirect(w, r, s.base.URL("/find"))
		return
	}

	next, ok := s.base.ValidNext(r.URL.Query().Get("next"))
	if !ok {
		next = s.base.URL("/find")
	}

	token, err := randomToken()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.setCookie(w, r, loginCookie, token, s.base.CookiePath(), loginCookieMaxAge)
	s.renderLogin(w, r, http.StatusOK, "", next, token)
}

func (s *Server) handleLoginPOST(w http.ResponseWriter, r *http.Request) {
	if !s.originOK(r) {
		s.forbidden(w, r, fmt.Sprintf("bad origin=%q host=%q scheme=%q",
			r.Header.Get("Origin"), r.Host, s.scheme(r)))
		return
	}
	if err := r.ParseForm(); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		s.badRequest(w, r, "invalid form")
		return
	}

	// Pre-auth CSRF: the token in the form must match the short-lived cookie.
	cookie, err := r.Cookie(loginCookie)
	formToken := r.PostFormValue("csrf")
	if err != nil || !tokenEqual(cookie.Value, formToken) {
		s.forbidden(w, r, "csrf")
		return
	}

	username := r.PostFormValue("username")
	next, ok := s.base.ValidNext(r.PostFormValue("next"))
	if !ok {
		next = s.base.URL("/find")
	}

	keys := s.loginKeys(r, username)
	for _, key := range keys {
		if retry, allowed := s.limiter.Allow(key); !allowed {
			seconds := int(retry.Seconds())
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", fmt.Sprintf("%d", seconds))
			s.renderLogin(w, r, http.StatusTooManyRequests,
				fmt.Sprintf("Too many attempts. Try again in %d s.", seconds), next, formToken)
			return
		}
	}

	canonical, valid := s.verifyLogin(username, r.PostFormValue("password"))
	if !valid {
		for _, key := range keys {
			s.limiter.Failure(key)
		}
		log.Printf("[Torrentino]: web: login failed user=%q ip=%s request_id=%s",
			username, s.clientIP(r), requestID(r))
		s.renderLogin(w, r, http.StatusOK, "Invalid username or password", next, formToken)
		return
	}

	for _, key := range keys {
		s.limiter.Success(key)
	}
	token, session, err := s.store.Create(canonical)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	_ = session
	s.clearCookie(w, r, loginCookie)
	s.setCookie(w, r, sessionCookie, token, s.base.CookiePath(), common.Settings.WebSessionMaxAge())
	log.Printf("[Torrentino]: web: login ok user=%q ip=%s request_id=%s",
		canonical, s.clientIP(r), requestID(r))
	s.redirect(w, r, next)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, session *Session) {
	cookie, err := r.Cookie(sessionCookie)
	if err == nil {
		s.store.Delete(cookie.Value)
	}
	s.clearCookie(w, r, sessionCookie)
	log.Printf("[Torrentino]: web: logout user=%q ip=%s request_id=%s",
		session.User, s.clientIP(r), requestID(r))
	s.redirect(w, r, s.base.URL("/login"))
}

// sessionFromRequest returns the session when the cookie is valid, without
// requiring middleware. It is used by /login to avoid showing the form to an
// already authenticated user.
func (s *Server) sessionFromRequest(r *http.Request) *Session {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	session, ok := s.store.Get(cookie.Value)
	if !ok {
		return nil
	}
	return session
}

func (s *Server) setCookie(w http.ResponseWriter, r *http.Request, name, value, path string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     s.base.CookiePath(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) secureCookies(r *http.Request) bool {
	return common.Settings.Web.SecureCookies || r.TLS != nil || s.scheme(r) == "https"
}

func (s *Server) renderLogin(w http.ResponseWriter, r *http.Request, status int, errMsg, next, csrf string) {
	data := LoginData{
		Base:          s.base.Prefix(),
		Next:          next,
		CSRF:          csrf,
		Error:         errMsg,
		StaticVersion: s.staticVersion,
		LoginURL:      s.base.URL("/login"),
	}
	s.render(w, r, status, "login", data)
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("[Torrentino]: web: internal error request_id=%s: %v", requestID(r), err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func (s *Server) forbidden(w http.ResponseWriter, r *http.Request, reason string) {
	log.Printf("[Torrentino]: web: forbidden request_id=%s reason=%s", requestID(r), reason)
	http.Error(w, "forbidden", http.StatusForbidden)
}

func (s *Server) badRequest(w http.ResponseWriter, r *http.Request, reason string) {
	log.Printf("[Torrentino]: web: bad request request_id=%s reason=%s", requestID(r), reason)
	http.Error(w, "bad request", http.StatusBadRequest)
}
