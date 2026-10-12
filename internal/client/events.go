package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Event invalidates a snapshot; it never carries private observations or replay records.
type Event struct {
	Name string          `json:"event"`
	Data json.RawMessage `json:"data"`
}

var ErrWatchComplete = errors.New("watch complete")

func KnownEvent(name string) bool {
	switch name {
	case "ready", "room_created", "state_changed", "observer_changed", "started", "round_revealed", "settled":
		return true
	}
	return false
}

// Watch reconnects public SSE connections until ctx ends or receive stops it.
// Every ready notification requires a fresh state read: events are not replayed.
func (c *Client) Watch(ctx context.Context, room string, receive func(Event) error) error {
	return c.watch(ctx, room, false, receive)
}
func (c *Client) WatchObserver(ctx context.Context, room string, receive func(Event) error) error {
	if c.Token == "" {
		return errors.New("observer credential required")
	}
	return c.watch(ctx, room, true, receive)
}
func (c *Client) watch(ctx context.Context, room string, observer bool, receive func(Event) error) error {
	h := *c.HTTP
	h.Timeout = 0 // Ordinary requests retain their timeout; SSE is long-lived.
	delay := 2 * time.Second
	endpoint, credential := c.URL+"/api/events", ""
	if observer {
		endpoint, credential = c.URL+"/api/observer/events", c.Token
	}
	for {
		retry, err := watchStream(ctx, &h, endpoint, credential, room, &delay, receive)
		if errors.Is(err, ErrWatchComplete) {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !retry {
			return err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func watchStream(ctx context.Context, h *http.Client, endpoint, credential, room string, delay *time.Duration, receive func(Event) error) (bool, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(streamCtx, "GET", endpoint, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "text/event-stream")
	// Only the protected observer stream receives a credential.
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	idle := time.AfterFunc(45*time.Second, cancel)
	defer idle.Stop()
	resp, err := h.Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode >= 500 || resp.StatusCode == 408 || resp.StatusCode == 429, fmt.Errorf("events HTTP %d", resp.StatusCode)
	}
	if strings.Split(resp.Header.Get("Content-Type"), ";")[0] != "text/event-stream" {
		return false, errors.New("events response is not text/event-stream")
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 64<<10)
	name := "message"
	var data []string
	size, lineNumber := 0, 0
	for scanner.Scan() {
		idle.Reset(45 * time.Second)
		line := scanner.Text()
		if lineNumber == 0 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		lineNumber++
		if line == "" {
			if len(data) > 0 && KnownEvent(name) {
				raw := json.RawMessage(strings.Join(data, "\n"))
				var notification struct {
					RoomID string `json:"room_id"`
				}
				if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &notification) != nil {
					return false, errors.New("invalid event JSON")
				}
				if room == "" || name == "ready" || notification.RoomID == room {
					if err := receive(Event{name, raw}); err != nil {
						return false, err
					}
				}
			}
			name, data, size = "message", nil, 0
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			name = value
		case "data":
			size += len(value) + 1
			if size > 64<<10 {
				return false, errors.New("event exceeds 64 KiB")
			}
			data = append(data, value)
		case "retry":
			if ms, err := strconv.Atoi(value); err == nil && ms >= 100 && ms <= 30000 {
				*delay = time.Duration(ms) * time.Millisecond
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return true, err
	}
	return true, io.EOF
}
