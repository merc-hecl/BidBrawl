package server

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"bidbrawl/internal/game"
	bolt "go.etcd.io/bbolt"
)

func persistentServer(t *testing.T, path string) *Server {
	t.Helper()
	g, err := game.New(2)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(g.GameConfig(), g.CardConfig(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if len(s.rooms) == 0 {
		rm := testTable(g).rooms["table"]
		if err := s.store.saveRoom(rm); err != nil {
			t.Fatal(err)
		}
		s.rooms["table"] = rm
	}
	s.EnableWebUI()
	return s
}

func responseStrings(t *testing.T, code int, body []byte, expected int) map[string]string {
	t.Helper()
	if code != expected {
		t.Fatalf("HTTP %d, expected %d", code, expected)
	}
	var response map[string]string
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestIdentitySurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	firstToken, firstCredential := accountSession(t, s, "Atlas")
	secondToken, secondCredential := accountSession(t, s, "Echo")
	code, body := request(t, s, "GET", "/api/me", firstToken, "")
	first := responseStrings(t, code, body, 200)
	code, body = request(t, s, "GET", "/api/me", secondToken, "")
	second := responseStrings(t, code, body, 200)
	if len(first["account_id"]) != 32 || first["account_id"] == second["account_id"] {
		t.Fatal("identities are not distinct permanent IDs")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{firstCredential, secondCredential, firstToken, secondToken, s.HostToken} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatal("database contains a plaintext credential or session")
		}
	}
	restarted := persistentServer(t, path)
	for i, cred := range []string{firstCredential, secondCredential} {
		code, body = request(t, restarted, "POST", "/api/login", "", `{"credential":"`+cred+`"}`)
		logged := responseStrings(t, code, body, 200)
		expected := []map[string]string{first, second}[i]
		if logged["account_id"] != expected["account_id"] || logged["name"] != expected["name"] {
			t.Fatal("restart changed identity")
		}
	}
	for _, old := range []string{firstToken, secondToken} {
		if code, _ = request(t, restarted, "GET", "/api/me", old, ""); code != 401 {
			t.Fatal("session survived restart")
		}
	}
	if code, _ = request(t, restarted, "POST", "/api/register", restarted.HostToken, `{"name":"Atlas"}`); code != 409 {
		t.Fatal("restart released registered name")
	}
	if code, _ = request(t, restarted, "GET", "/api/accounts", s.HostToken, ""); code != 403 {
		t.Fatal("old host token survived restart")
	}
}

func TestIdentityAdministrationAndRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	a, credential := accountSession(t, s, "Atlas")
	b, _ := accountSession(t, s, "Echo")
	code, body := request(t, s, "GET", "/api/me", a, "")
	id := responseStrings(t, code, body, 200)["account_id"]
	for _, actor := range []string{"", a, b, "fake"} {
		for _, endpoint := range []struct{ method, path, body string }{
			{"POST", "/api/register", `{"name":"Stolen"}`},
			{"GET", "/api/accounts", ""},
			{"POST", "/api/accounts/" + id + "/credential", ""},
		} {
			if code, _ = request(t, s, endpoint.method, endpoint.path, actor, endpoint.body); code != 403 {
				t.Fatal("identity administration allowed without host token")
			}
		}
	}
	for _, fakeCredential := range []string{"Atlas", id} {
		if code, _ = request(t, s, "POST", "/api/login", "", `{"credential":"`+fakeCredential+`"}`); code != 401 {
			t.Fatal("public name or ID allowed impersonation")
		}
	}
	code, body = request(t, s, "GET", "/api/accounts", s.HostToken, "")
	var list struct {
		Accounts []account `json:"accounts"`
	}
	if code != 200 || json.Unmarshal(body, &list) != nil || len(list.Accounts) != 2 || bytes.Contains(body, []byte(credential)) || bytes.Contains(body, []byte("credential")) {
		t.Fatal("account list incorrect or contains credentials")
	}
	if code, _ = request(t, s, "POST", "/api/accounts/missing/credential", s.HostToken, ""); code != 404 {
		t.Fatal("unknown account rotation accepted")
	}
	code, body = request(t, s, "POST", "/api/login", "", `{"credential":"`+credential+`"}`)
	a2 := responseStrings(t, code, body, 200)["token"]
	if code, _ = request(t, s, "POST", "/api/join", a, `{"cards":[]}`); code != 201 {
		t.Fatal("join failed")
	}
	code, body = request(t, s, "POST", "/api/accounts/"+id+"/credential", s.HostToken, "")
	rotated := responseStrings(t, code, body, 200)
	if rotated["account_id"] != id || rotated["credential"] == credential {
		t.Fatal("rotation changed identity or reused credential")
	}
	for _, old := range []string{a, a2} {
		if code, _ = request(t, s, "GET", "/api/me", old, ""); code != 401 {
			t.Fatal("rotation left old session active")
		}
	}
	if code, _ = request(t, s, "POST", "/api/login", "", `{"credential":"`+credential+`"}`); code != 401 {
		t.Fatal("rotation left old credential active")
	}
	if code, _ = request(t, s, "GET", "/api/me", b, ""); code != 200 {
		t.Fatal("rotation revoked another account")
	}
	code, body = request(t, s, "POST", "/api/login", "", `{"credential":"`+rotated["credential"]+`"}`)
	newToken := responseStrings(t, code, body, 200)["token"]
	code, body = request(t, s, "GET", "/api/state", newToken, "")
	var view game.View
	if code != 200 || json.Unmarshal(body, &view) != nil || view.Self == nil || view.Self.ID != "p1" {
		t.Fatal("rotation lost room membership")
	}
	_ = s.Close()
	s = persistentServer(t, path)
	if code, _ = request(t, s, "POST", "/api/login", "", `{"credential":"`+credential+`"}`); code != 401 {
		t.Fatal("restart restored revoked credential")
	}
	code, body = request(t, s, "POST", "/api/login", "", `{"credential":"`+rotated["credential"]+`"}`)
	if responseStrings(t, code, body, 200)["account_id"] != id {
		t.Fatal("rotated identity did not survive restart")
	}
}

func TestIdentityWriteFailureDoesNotChangeMemory(t *testing.T) {
	s := persistentServer(t, filepath.Join(t.TempDir(), "accounts.db"))
	token, credential := accountSession(t, s, "Atlas")
	code, body := request(t, s, "GET", "/api/me", token, "")
	id := responseStrings(t, code, body, 200)["account_id"]
	_ = s.Close() // Force transaction failures, with the preexisting cache intact.
	if code, _ = request(t, s, "POST", "/api/register", s.HostToken, `{"name":"Echo"}`); code != 500 || len(s.accounts) != 1 {
		t.Fatal("failed persistence created an in-memory identity")
	}
	if code, _ = request(t, s, "POST", "/api/accounts/"+id+"/credential", s.HostToken, ""); code != 500 {
		t.Fatal("failed persistence reported successful rotation")
	}
	if code, _ = request(t, s, "GET", "/api/me", token, ""); code != 200 {
		t.Fatal("failed rotation revoked session")
	}
	if code, _ = request(t, s, "POST", "/api/login", "", `{"credential":"`+credential+`"}`); code != 200 {
		t.Fatal("failed rotation revoked credential")
	}
}

func TestIdentityConcurrentRegistration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	var wg sync.WaitGroup
	statuses := make(chan int, 12)
	for range 12 {
		wg.Go(func() {
			code, _ := request(t, s, "POST", "/api/register", s.HostToken, `{"name":"Atlas"}`)
			statuses <- code
		})
	}
	wg.Wait()
	close(statuses)
	created := 0
	for code := range statuses {
		if code == 201 {
			created++
		} else if code != 409 {
			t.Fatal("unexpected concurrent registration status", code)
		}
	}
	if created != 1 {
		t.Fatal("concurrent registrations did not create exactly one identity")
	}
	_ = s.Close()
	if len(persistentServer(t, path).accounts) != 1 {
		t.Fatal("concurrent identity was not persisted")
	}
}

func TestIdentityDatabaseFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.db")
	s := persistentServer(t, path)
	g, _ := game.New(2)
	if other, err := Open(g.GameConfig(), g.CardConfig(), path); err == nil {
		_ = other.Close()
		t.Fatal("second host opened the same identity database")
	}
	_ = s.Close()
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("metadata")).Put([]byte("schema_version"), []byte("99"))
	})
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Open(g.GameConfig(), g.CardConfig(), path); err == nil {
		_ = other.Close()
		t.Fatal("unsupported schema silently replaced")
	}
	corrupt := filepath.Join(t.TempDir(), "corrupt.db")
	if err = os.WriteFile(corrupt, []byte(strings.Repeat("broken", 4096)), 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := Open(g.GameConfig(), g.CardConfig(), corrupt); err == nil {
		_ = other.Close()
		t.Fatal("corrupt database silently replaced")
	}
}
