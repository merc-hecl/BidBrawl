package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bidbrawl/internal/game"
	"bidbrawl/internal/server"
	bolt "go.etcd.io/bbolt"
)

func TestBrowserAddress(t *testing.T) {
	for _, address := range []string{"0.0.0.0:8787", "[::]:8787", "127.0.0.1:8787"} {
		a, _ := net.ResolveTCPAddr("tcp", address)
		if got := browserAddress(a); got != "http://127.0.0.1:8787" {
			t.Fatal(got)
		}
	}
}

func claimLaunch(address string) error {
	u, err := url.Parse(address)
	if err != nil {
		return err
	}
	fragment, _ := url.ParseQuery(u.Fragment)
	u.Fragment = ""
	u.Path = "/api/observer/bootstrap"
	body, _ := json.Marshal(map[string]string{"code": fragment.Get("launch")})
	response, err := http.Post(u.String(), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || len(response.Cookies()) != 2 {
		return fmt.Errorf("launch bootstrap: %d", response.StatusCode)
	}
	return nil
}

func TestHostAutomaticallyLaunchesAuthorizedUI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	launched := make(chan error, 1)
	err := runWithBrowser(ctx, []string{"host", "-webui", "-listen", "127.0.0.1:0", "-data-dir", t.TempDir()}, func(address string) error {
		err := claimLaunch(address)
		launched <- err
		cancel()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-launched:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("host did not open browser by default")
	}
}

func TestHostReportsLockedDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.db")
	g, _ := game.New(2)
	s, err := server.Open(g.GameConfig(), g.CardConfig(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	called := false
	err = runWithBrowser(context.Background(), []string{"host", "-webui", "-listen", "127.0.0.1:0", "-data-dir", dir}, func(string) error {
		called = true
		return nil
	})
	if !errors.Is(err, bolt.ErrTimeout) {
		t.Fatalf("expected database lock timeout, got %v", err)
	}
	for _, want := range []string{fmt.Sprintf("%q", path), "其他进程占用", "先停止原宿主", "更换监听端口不会释放数据库锁"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in error: %v", want, err)
		}
	}
	if called {
		t.Fatal("failed host opened browser")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := server.Open(g.GameConfig(), g.CardConfig(), path)
	if err != nil {
		t.Fatalf("database lock was not released after closing host: %v", err)
	}
	defer reopened.Close()
}

func TestUICommandRequiresHostAndOpensReadOnlySession(t *testing.T) {
	g, _ := game.New(2)
	s := server.New(g.GameConfig(), g.CardConfig())
	s.EnableWebUI()
	h := httptest.NewServer(s)
	defer h.Close()
	called := false
	opener := func(address string) error {
		called = true
		u, _ := url.Parse(address)
		if u.Query().Get("room") != "" {
			t.Fatal("unexpected default room")
		}
		return claimLaunch(address)
	}
	if err := runWithBrowser(context.Background(), []string{"ui", "-url", h.URL, "-token", "player"}, opener); err == nil || called {
		t.Fatal("unauthorized UI launch")
	}
	if err := runWithBrowser(context.Background(), []string{"ui", "-url", h.URL, "-token", s.HostToken}, opener); err != nil || !called {
		t.Fatal("UI launch failed", err)
	}
}
