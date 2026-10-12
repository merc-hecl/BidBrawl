// Package client is a thin game HTTP client. It makes no model calls or decisions.
package client

import (
	"bidbrawl/internal/game"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	URL, Token string
	HTTP       *http.Client
}

func New(url, token string) *Client {
	return &Client{strings.TrimRight(url, "/"), token, &http.Client{Timeout: 15 * time.Second}}
}
func (c *Client) Request(ctx context.Context, method, path string, body, out any) error {
	var input io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		input = bytes.NewReader(b)
	}
	r, e := http.NewRequestWithContext(ctx, method, c.URL+path, input)
	if e != nil {
		return e
	}
	r.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		r.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, e := c.HTTP.Do(r)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	const maxResponse = 4 << 20
	data, e := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if e != nil {
		return e
	}
	if len(data) > maxResponse {
		return fmt.Errorf("game response exceeds %d bytes", maxResponse)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}
func RoomPath(path, room string) string {
	return path + "?room=" + url.QueryEscape(room)
}

type Observation struct {
	game.View
	RoomID   string `json:"room_id"`
	RoomName string `json:"room_name"`
}

func (c *Client) Observe(ctx context.Context, room string) (Observation, error) {
	var v Observation
	e := c.Request(ctx, "GET", RoomPath("/api/state", room), nil, &v)
	return v, e
}
