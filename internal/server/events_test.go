package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bidbrawl/internal/game"
)

type receivedEvent struct{ kind, data string }

func openEvents(t *testing.T, url string) (<-chan receivedEvent, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", url+"/api/events", nil)
	// Reconnecting clients must get ready, even with an old Last-Event-ID.
	req.Header.Set("Last-Event-ID", "previous-host:999")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if response.StatusCode != 200 || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") || response.Header.Get("Cache-Control") != "no-store" {
		cancel()
		response.Body.Close()
		t.Fatal("invalid event response")
	}
	events := make(chan receivedEvent, 64)
	go func() {
		defer close(events)
		scanner := bufio.NewScanner(response.Body)
		var event receivedEvent
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" && event.kind != "" {
				events <- event
				event = receivedEvent{}
			}
			if strings.HasPrefix(line, "event: ") {
				event.kind = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				event.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return events, func() { cancel(); response.Body.Close() }
}

func nextEvent(t *testing.T, events <-chan receivedEvent, kind string) roomEvent {
	t.Helper()
	select {
	case received, ok := <-events:
		if !ok || received.kind != kind {
			t.Fatalf("event %q, want %q", received.kind, kind)
		}
		var event roomEvent
		if json.Unmarshal([]byte(received.data), &event) != nil {
			t.Fatal("bad event JSON")
		}
		// Payload is only invalidation metadata, never an observation or credential.
		var fields map[string]any
		_ = json.Unmarshal([]byte(received.data), &fields)
		for key := range fields {
			if key != "room_id" && key != "version" && key != "rooms" && key != "results" {
				t.Fatalf("unexpected public event field %s", key)
			}
		}
		return event
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for %s", kind)
	}
	return roomEvent{}
}

func TestSSELifecyclePrivacyReconnectAndShutdown(t *testing.T) {
	s := persistentServer(t, filepath.Join(t.TempDir(), "accounts.db"))
	a, _ := accountSession(t, s, "Atlas")
	b, _ := accountSession(t, s, "Echo")
	h := httptest.NewUnstartedServer(s)
	h.Config.WriteTimeout = 100 * time.Millisecond
	h.Start()
	defer h.Close()
	events, closeStream := openEvents(t, h.URL)
	defer closeStream()
	nextEvent(t, events, "ready")
	created := checkedRequest(t, s, "POST", "/api/create", a, `{"name":"Other room","players":2}`, 201)
	var roomInfo map[string]any
	_ = json.Unmarshal(created, &roomInfo)
	createdEvent := nextEvent(t, events, "room_created")
	if createdEvent.RoomID != roomInfo["room_id"] || createdEvent.Version != 1 || !createdEvent.Rooms {
		t.Fatal("invalid room creation notification")
	}
	// Stream must survive the ordinary response timeout.
	time.Sleep(150 * time.Millisecond)
	for _, token := range []string{a, b} {
		checkedRequest(t, s, "POST", "/api/join", token, `{"cards":["largest_price"]}`, 201)
		e := nextEvent(t, events, "state_changed")
		if !e.Rooms || e.RoomID != "table" {
			t.Fatal("missing lobby invalidation")
		}
	}
	checkedRequest(t, s, "POST", "/api/start", s.HostToken, "", 200)
	nextEvent(t, events, "started")
	checkedRequest(t, s, "POST", "/api/card", a, `{"reason":"protocol test","round":1,"card":"largest_price"}`, 200)
	checkedRequest(t, s, "POST", "/api/bid", a, `{"reason":"protocol test","round":1,"amount":987654}`, 200)
	e := nextEvent(t, events, "state_changed") // No card event may precede this.
	if e.Rooms || e.Results {
		t.Fatal("pending bid changed metadata")
	}
	checkedRequest(t, s, "POST", "/api/bid", b, `{"reason":"protocol test","round":1,"amount":600000}`, 200)
	nextEvent(t, events, "round_revealed")
	for round := 2; round <= game.Rounds; round++ {
		checkedRequest(t, s, "POST", "/api/bid", a, fmt.Sprintf(`{"reason":"protocol test","round":%d,"amount":100}`, round), 200)
		nextEvent(t, events, "state_changed")
		checkedRequest(t, s, "POST", "/api/bid", b, fmt.Sprintf(`{"reason":"protocol test","round":%d,"amount":101}`, round), 200)
		kind := "round_revealed"
		if round == game.Rounds {
			kind = "settled"
		}
		e = nextEvent(t, events, kind)
	}
	if !e.Results || !e.Rooms {
		t.Fatal("settlement did not invalidate results")
	}
	closeStream()
	reconnected, stop := openEvents(t, h.URL)
	defer stop()
	nextEvent(t, reconnected, "ready")
	state := checkedRequest(t, s, "GET", "/api/state", "", "", 200)
	var view game.View
	_ = json.Unmarshal(state, &view)
	if view.Phase != "settled" || len(view.History) != game.Rounds {
		t.Fatal("reconnect cannot recover current state")
	}
	s.CloseEvents()
	select {
	case _, ok := <-reconnected:
		if ok {
			t.Fatal("unexpected event after shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("stream prevented shutdown")
	}
}

func TestEventsOnlyCommittedChangesAndSlowSubscriber(t *testing.T) {
	s := persistentServer(t, filepath.Join(t.TempDir(), "accounts.db"))
	a, _ := accountSession(t, s, "Atlas")
	ch := make(chan roomEvent, 1)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	if err := s.store.db.Close(); err != nil {
		t.Fatal(err)
	}
	checkedRequest(t, s, "POST", "/api/join", a, `{"cards":[]}`, 500)
	checkedRequest(t, s, "POST", "/api/create", a, `{"name":"Failed","players":2}`, 500)
	select {
	case <-ch:
		t.Fatal("uncommitted change was broadcast")
	default:
	}
	s.mu.Lock()
	s.publishEvent(roomEvent{Kind: "state_changed"})
	s.publishEvent(roomEvent{Kind: "state_changed"})
	if len(s.subscribers) != 0 {
		t.Fatal("slow observer retained")
	}
	s.mu.Unlock()
	<-ch
	if _, ok := <-ch; ok {
		t.Fatal("overflow did not close stream")
	}
}

func TestSSEDisconnectReleasesSubscriber(t *testing.T) {
	g, _ := game.New(2)
	s := testTable(g)
	defer s.Close()
	h := httptest.NewServer(s)
	defer h.Close()
	events, stop := openEvents(t, h.URL)
	nextEvent(t, events, "ready")
	stop()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		count := len(s.subscribers)
		s.mu.Unlock()
		if count == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("disconnected observer was retained")
}
