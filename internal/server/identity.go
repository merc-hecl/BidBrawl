package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"
)

func (s *Server) requireHost(w http.ResponseWriter, r *http.Request) bool {
	if subtle.ConstantTimeCompare([]byte(token(r)), []byte(s.HostToken)) != 1 {
		fail(w, http.StatusForbidden, errors.New("host token required"))
		return false
	}
	return true
}

func (s *Server) saveIdentity(w http.ResponseWriter, a *account, hash [32]byte) bool {
	if s.store != nil {
		if err := s.store.saveAccount(a, hash); err != nil {
			log.Printf("save identity: %v", err)
			fail(w, http.StatusInternalServerError, errors.New("could not persist identity"))
			return false
		}
	}
	return true
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if !s.requireHost(w, r) {
		return
	}
	var v struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &v) {
		return
	}
	v.Name = strings.TrimSpace(v.Name)
	if !validName(v.Name) {
		fail(w, 400, errors.New("name must contain 1 to 32 non-control characters"))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accounts[v.Name] != nil {
		fail(w, 409, errors.New("name already registered"))
		return
	}
	a := &account{Name: v.Name}
	for a.ID == "" || s.accountsByID[a.ID] != nil {
		a.ID = secret()[:32]
	}
	credential := secret()
	hash := sha256.Sum256([]byte(credential))
	if !s.saveIdentity(w, a, hash) {
		return
	}
	s.accounts[a.Name] = a
	s.accountsByID[a.ID] = a
	s.credentials[hash] = a
	s.credentialHashes[a.ID] = hash
	reply(w, 201, map[string]string{"account_id": a.ID, "name": a.Name, "credential": credential})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Credential string `json:"credential"`
	}
	if !decode(w, r, &v) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.credentials[sha256.Sum256([]byte(v.Credential))]
	if a == nil {
		fail(w, 401, errors.New("invalid login credential"))
		return
	}
	t := secret()
	s.sessions[t] = a
	reply(w, 200, map[string]string{"account_id": a.ID, "name": a.Name, "token": t})
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	if !s.requireHost(w, r) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]account, 0, len(s.accountsByID))
	for _, a := range s.accountsByID {
		list = append(list, *a)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	reply(w, 200, map[string]any{"accounts": list})
}

// A credential can change without changing the permanent ID or room membership.
// Persist the new digest before revoking the old credential and all its sessions.
func (s *Server) rotateCredential(w http.ResponseWriter, r *http.Request) {
	if !s.requireHost(w, r) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.accountsByID[r.PathValue("id")]
	if a == nil {
		fail(w, 404, errors.New("account not found"))
		return
	}
	credential := secret()
	hash := sha256.Sum256([]byte(credential))
	if !s.saveIdentity(w, a, hash) {
		return
	}
	delete(s.credentials, s.credentialHashes[a.ID])
	s.credentials[hash] = a
	s.credentialHashes[a.ID] = hash
	for t, actor := range s.sessions {
		if actor.ID == a.ID {
			delete(s.sessions, t)
		}
	}
	reply(w, 200, map[string]string{"account_id": a.ID, "name": a.Name, "credential": credential})
}
