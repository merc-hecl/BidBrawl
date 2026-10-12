package server

import (
	"errors"
	"net/http"
)

// Invitations identify a table; they never replace a player's own login.
func (s *Server) invite(w http.ResponseWriter, r *http.Request) {
	rm := s.getRoom(w, r)
	if rm == nil {
		return
	}
	manager := s.isRoomManager(r)
	var a *account
	if !manager {
		a = s.requireActor(w, r)
		if a == nil {
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !manager && (s.sessions[token(r)] != a || rm.owner != a.ID) {
		fail(w, 403, errors.New("room owner required"))
		return
	}
	view, err := publicView(rm.game)
	if err != nil {
		roomFailure(w, err)
		return
	}
	if view.Phase != "lobby" {
		fail(w, 409, errors.New("invitations require a lobby"))
		return
	}
	reply(w, 200, map[string]string{"room_id": rm.id, "invite_path": "/invite?room=" + rm.id})
}
