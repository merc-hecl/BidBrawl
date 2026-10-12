package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIWatchNDJSONReadyFilteringAndOnce(t *testing.T) {
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: ready\ndata: {}\n\nevent: state_changed\ndata: {\"room_id\":\"table\"}\n\nevent: started\ndata: {\"room_id\":\"table\"}\n\n")
	}))
	defer host.Close()
	f, err := os.Create(filepath.Join(t.TempDir(), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	previous := os.Stdout
	os.Stdout = f
	err = run(context.Background(), []string{"watch", "-url", host.URL, "-room", "table", "-event", "started", "-once", "-timeout", "1s"})
	os.Stdout = previous
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(f.Name())
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected ready and started, got %d lines", len(lines))
	}
	for i, name := range []string{"ready", "started"} {
		var event map[string]any
		if json.Unmarshal([]byte(lines[i]), &event) != nil || event["event"] != name {
			t.Fatalf("bad event %s", lines[i])
		}
	}
}

func TestCLIWatchInvalidOptions(t *testing.T) {
	for _, args := range [][]string{{"watch", "-event", "private_hint"}, {"watch", "-timeout", "-1s"}} {
		if run(context.Background(), args) == nil {
			t.Fatal("invalid watch options accepted")
		}
	}
}

func TestCLIObserverWatchAndGlobalSubscription(t *testing.T) {
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/observer/events" || r.Header.Get("Authorization") != "Bearer readonly" {
			t.Error("observer stream credential/path mismatch")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: ready\ndata: {}\n\nevent: observer_changed\ndata: {\"room_id\":\"new-room\"}\n\n")
	}))
	defer host.Close()
	f, err := os.Create(filepath.Join(t.TempDir(), "observer-events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	previous := os.Stdout
	os.Stdout = f
	err = run(context.Background(), []string{"watch", "-url", host.URL, "-observer", "-observer-token", "readonly", "-event", "observer_changed", "-once", "-timeout", "1s"})
	os.Stdout = previous
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(f.Name())
	if !strings.Contains(string(data), "observer_changed") || !strings.Contains(string(data), "new-room") {
		t.Fatal("global observer event dropped")
	}
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer denied.Close()
	if err = run(context.Background(), []string{"watch", "-url", denied.URL, "-observer", "-observer-token", "player", "-once", "-timeout", "1s"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatal("unauthorized observer watch accepted", err)
	}
}
