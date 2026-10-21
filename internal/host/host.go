// Package host runs the same game service for the CLI and desktop application.
package host

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/game"
	"bidbrawl/internal/hints"
	"bidbrawl/internal/server"
)

type Options struct {
	Listen      string `json:"listen"`
	DataDir     string `json:"data_dir"`
	CardsConfig string `json:"cards_config,omitempty"`
	ItemsConfig string `json:"items_config,omitempty"`
	GameConfig  string `json:"game_config,omitempty"`
	HintsConfig string `json:"hints_config,omitempty"`
	WebUI       bool   `json:"-"`
}

func (o *Options) Flags(f *flag.FlagSet) {
	if o.Listen == "" {
		o.Listen = "127.0.0.1:8787"
	}
	if o.DataDir == "" {
		o.DataDir = os.Getenv("BIDBRAWL_DATA_DIR")
	}
	f.StringVar(&o.Listen, "listen", o.Listen, "listen address; use 0.0.0.0:8787 for LAN access")
	f.StringVar(&o.DataDir, "data-dir", o.DataDir, "account, room and game data directory; defaults to BidBrawl in the user configuration directory")
	f.StringVar(&o.CardsConfig, "cards-config", o.CardsConfig, "skill-card JSON configuration path; uses built-in defaults if omitted")
	f.StringVar(&o.ItemsConfig, "items-config", o.ItemsConfig, "物品目录 JSON 路径，省略使用内置目录")
	f.StringVar(&o.GameConfig, "game-config", o.GameConfig, "game JSON configuration path; uses built-in defaults if omitted")
	f.StringVar(&o.HintsConfig, "hints-config", o.HintsConfig, "public-hint JSON configuration path; uses built-in defaults if omitted")
}

func DatabasePath(dataDir string) (string, error) {
	if dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("find identity directory: %w", err)
		}
		dataDir = filepath.Join(base, "BidBrawl")
	}
	return filepath.Abs(filepath.Join(dataDir, "accounts.db"))
}

func BrowserAddress(address net.Addr) string {
	name, port, _ := net.SplitHostPort(address.String())
	if ip := net.ParseIP(name); ip != nil && ip.IsUnspecified() {
		name = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(name, port)
}

func LaunchURL(address, room, code string) string {
	u, _ := url.Parse(address)
	if room != "" {
		q := u.Query()
		q.Set("room", room)
		u.RawQuery = q.Encode()
	}
	u.Fragment = "launch=" + url.QueryEscape(code)
	return u.String()
}

type Service struct {
	Address      string
	DatabasePath string
	server       *server.Server
}

// LaunchURL creates a one-use human session. No administrator credential is
// passed to the renderer, its storage, or the CLI's player authentication.
func (s *Service) LaunchURL() string {
	return LaunchURL(s.Address, "", s.server.NewObserverLaunchCode())
}
func (s *Service) HostToken() string { return s.server.HostToken }

func Run(ctx context.Context, o Options, ready func(*Service)) error {
	cfg := cards.Default()
	if o.CardsConfig != "" {
		loaded, err := cards.Load(o.CardsConfig)
		if err != nil {
			return err
		}
		cfg = loaded
	}
	geometry := game.DefaultConfig()
	if o.GameConfig != "" {
		loaded, err := game.LoadConfig(o.GameConfig)
		if err != nil {
			return err
		}
		geometry = loaded
	}
	hintConfig := hints.Default()
	if o.HintsConfig != "" {
		loaded, err := hints.Load(o.HintsConfig)
		if err != nil {
			return err
		}
		hintConfig = loaded
	}
	if o.ItemsConfig != "" {
		items, err := game.LoadCatalog(o.ItemsConfig)
		if err != nil {
			return err
		}
		geometry.Catalog = items
		if err := geometry.Validate(); err != nil {
			return err
		}
	}
	databasePath, err := DatabasePath(o.DataDir)
	if err != nil {
		return err
	}
	s, err := server.OpenWithHints(geometry, cfg, hintConfig, databasePath)
	if err != nil {
		return err
	}
	defer s.Close()
	if o.WebUI {
		s.EnableWebUI()
	}
	ln, err := net.Listen("tcp", o.Listen)
	if err != nil {
		return err
	}
	defer ln.Close()
	h := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	hostCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopped := make(chan error, 1)
	go func() {
		<-hostCtx.Done()
		s.CloseEvents()
		shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		err := h.Shutdown(shutdown)
		if err != nil {
			_ = h.Close()
		}
		stopped <- err
	}()
	if ready != nil {
		go ready(&Service{Address: BrowserAddress(ln.Addr()), DatabasePath: databasePath, server: s})
	}
	err = h.Serve(ln)
	cancel()
	shutdownErr := <-stopped
	if errors.Is(err, http.ErrServerClosed) {
		return shutdownErr
	}
	return errors.Join(err, shutdownErr)
}
