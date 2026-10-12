package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchLongStreamFiltersRoomsAndKeepsCredentialsPrivate(t *testing.T) {
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events" || r.Header.Get("Authorization") != "" {
			t.Error("watch used a private token or wrong endpoint")
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		fmt.Fprint(w, "\ufeff: heartbeat\r\nevent: ready\r\ndata: {}\r\n\r\n")
		w.(http.Flusher).Flush()
		time.Sleep(60 * time.Millisecond)
		fmt.Fprint(w, "event: started\ndata: {\ndata: \"room_id\":\"other\"}\n\nevent: started\ndata: {\ndata: \"room_id\":\"a&b\",\"version\":2}\n\n")
	}))
	defer host.Close()
	c := New(host.URL, "private-token")
	c.HTTP.Timeout = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var got []string
	err := c.Watch(ctx, "a&b", func(event Event) error {
		got = append(got, event.Name)
		if event.Name == "started" {
			return ErrWatchComplete
		}
		return nil
	})
	if err != nil || !reflect.DeepEqual(got, []string{"ready", "started"}) {
		t.Fatalf("events %v: %v", got, err)
	}
	if c.HTTP.Timeout != 10*time.Millisecond {
		t.Fatal("ordinary HTTP timeout changed")
	}
}

func TestWatchReconnectResyncAndCancel(t *testing.T) {
	var connections atomic.Int32
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "retry: 100\nevent: ready\ndata: {}\n\n")
		if connections.Add(1) > 1 {
			fmt.Fprint(w, "event: settled\ndata: {\"room_id\":\"table\"}\n\n")
		}
	}))
	defer host.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var got []string
	err := New(host.URL, "").Watch(ctx, "table", func(event Event) error {
		got = append(got, event.Name)
		if event.Name == "settled" {
			return ErrWatchComplete
		}
		return nil
	})
	if err != nil || !reflect.DeepEqual(got, []string{"ready", "ready", "settled"}) {
		t.Fatalf("reconnect events %v: %v", got, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	err = New(host.URL, "").Watch(ctx, "table", func(event Event) error { cancel(); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestWatchProtocolAndConsumerErrorsStop(t *testing.T) {
	for _, sample := range []struct {
		name, content, body string
		status              int
	}{
		{"bad type", "application/json", "{}", 200},
		{"missing", "text/event-stream", "", 404},
		{"bad JSON", "text/event-stream", "event: ready\ndata: null\n\n", 200},
		{"consumer", "text/event-stream", "event: ready\ndata: {}\n\n", 200},
	} {
		t.Run(sample.name, func(t *testing.T) {
			host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", sample.content)
				w.WriteHeader(sample.status)
				fmt.Fprint(w, sample.body)
			}))
			defer host.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := New(host.URL, "").Watch(ctx, "table", func(Event) error { return io.ErrClosedPipe })
			if err == nil || errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("did not fail immediately: %v", err)
			}
		})
	}
}
