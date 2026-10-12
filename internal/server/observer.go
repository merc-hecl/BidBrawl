package server

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"time"
)

const observerCookie = "bidbrawl_observer"
const uiCookie = "bidbrawl_ui"

// Human room management is separate from read-only observation and player login.
func (s *Server) isRoomManager(r *http.Request) bool {
	if subtle.ConstantTimeCompare([]byte(token(r)), []byte(s.HostToken)) == 1 {
		return true
	}
	cookie, err := r.Cookie(uiCookie)
	return err == nil && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.UIToken)) == 1
}

// This credential grants read-only observer access, never player/admin actions.
func (s *Server) isObserver(r *http.Request) bool {
	value := token(r)
	if value == "" {
		cookie, err := r.Cookie(observerCookie)
		if err == nil {
			value = cookie.Value
		}
	}
	return subtle.ConstantTimeCompare([]byte(value), []byte(s.ObserverToken)) == 1
}
func (s *Server) requireObserver(w http.ResponseWriter, r *http.Request) bool {
	if !s.isObserver(r) {
		fail(w, 401, errors.New("observer credential required"))
		return false
	}
	return true
}
func observerSessionCookie(value string, r *http.Request) *http.Cookie {
	return &http.Cookie{Name: observerCookie, Value: value, Path: "/api/observer", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode}
}

// Headless human observers receive a read-only credential, never player/admin access.
func (s *Server) observerLogin(w http.ResponseWriter, r *http.Request) {
	if s.requireHost(w, r) {
		reply(w, 200, map[string]string{"observer_token": s.ObserverToken})
	}
}

// NewObserverLaunchCode grants one browser observation and room-management sessions.
// The host passes it in a URL fragment, never as a persistent credential.
func (s *Server) NewObserverLaunchCode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observerLaunchCode = secret()
	s.observerLaunchUntil = time.Now().Add(2 * time.Minute)
	return s.observerLaunchCode
}
func (s *Server) observerLaunch(w http.ResponseWriter, r *http.Request) {
	if !s.webUI {
		http.NotFound(w, r)
		return
	}
	if s.requireHost(w, r) {
		reply(w, 200, map[string]string{"code": s.NewObserverLaunchCode()})
	}
}
func (s *Server) observerBootstrap(w http.ResponseWriter, r *http.Request) {
	if !s.webUI {
		http.NotFound(w, r)
		return
	}
	var v struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &v) {
		return
	}
	s.mu.Lock()
	valid := s.observerLaunchCode != "" && time.Now().Before(s.observerLaunchUntil) && subtle.ConstantTimeCompare([]byte(v.Code), []byte(s.observerLaunchCode)) == 1
	if valid {
		s.observerLaunchCode = ""
	}
	s.mu.Unlock()
	if !valid {
		fail(w, 401, errors.New("invalid or expired UI launch code"))
		return
	}
	http.SetCookie(w, observerSessionCookie(s.ObserverToken, r))
	http.SetCookie(w, &http.Cookie{Name: uiCookie, Value: s.UIToken, Path: "/api", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
	reply(w, 200, map[string]bool{"authenticated": true})
}
func (s *Server) observerSession(w http.ResponseWriter, r *http.Request) {
	reply(w, 200, map[string]bool{"authenticated": s.isObserver(r), "manage_rooms": s.isRoomManager(r)})
}
func (s *Server) observerState(w http.ResponseWriter, r *http.Request) {
	if s.requireObserver(w, r) {
		s.stateFor(w, r, true)
	}
}
func (s *Server) observerEvents(w http.ResponseWriter, r *http.Request) {
	if s.requireObserver(w, r) {
		s.serveEvents(w, r, true)
	}
}
