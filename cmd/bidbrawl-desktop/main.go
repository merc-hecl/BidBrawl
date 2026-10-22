package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"bidbrawl/internal/host"
	"bidbrawl/internal/server"
	"github.com/egoist/mygo"
)

//go:embed web/*
var pages embed.FS

type Desktop struct {
	op         sync.Mutex
	options    host.Options
	configPath string
	window     *mygo.Window
	settings   *mygo.Window
	service    *host.Service
	cancel     context.CancelFunc
	done       chan error
	quitting   atomic.Bool
	approved   atomic.Bool
	loadError  error
}

func text(zh, en string) string {
	if strings.HasPrefix(mygo.App.Locale(), "zh") {
		return zh
	}
	return en
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	mygo.App.SetName("BidBrawl")
	mygo.App.SetLanguages("zh-CN", "en")
	base, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	d := &Desktop{configPath: filepath.Join(base, "BidBrawl", "desktop.json")}
	if path := os.Getenv("BIDBRAWL_DESKTOP_CONFIG"); path != "" {
		d.configPath = path
	}
	if data, err := os.ReadFile(d.configPath); err == nil {
		if err := json.Unmarshal(data, &d.options); err != nil {
			d.loadError = fmt.Errorf("read desktop settings: %w", err)
			d.options = host.Options{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		d.loadError = err
	}
	f := flag.NewFlagSet("BidBrawl", flag.ContinueOnError)
	d.options.Flags(f)
	if err := f.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected argument: %s", f.Arg(0))
	}
	d.options.WebUI = true
	mygo.Bind(d)
	frontend, _ := fs.Sub(pages, "web")
	mygo.SetFrontend(frontend)
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	mygo.App.OnSecondInstance(func([]string, string) { d.show() })
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			d.show()
		}
	})
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if d.approved.Load() {
			return
		}
		e.PreventDefault()
		d.show()
		if !d.quitting.CompareAndSwap(false, true) {
			return
		}
		go func() {
			defer d.quitting.Store(false)
			d.op.Lock()
			defer d.op.Unlock()
			if d.service != nil {
				accepted, err := confirmOnPage(d.window, "停止宿主并退出？", "Stop the host and quit?", "停止并退出", "Stop and quit")
				if err != nil || !accepted {
					return
				}
			}
			if err := d.stopLocked(); err != nil {
				mygo.Dialog.Error(text("停止宿主失败", "Could not stop the host"), err.Error())
				return
			}
			d.approved.Store(true)
			mygo.App.Quit()
		}()
	})
	mygo.App.WhenReady(func() {
		d.window = mygo.NewWindow(mygo.WindowOptions{Title: "BidBrawl", URL: "/", Width: 1280, Height: 860,
			MinWidth: 740, MinHeight: 540, StateKey: "main", BackgroundColor: "light-dark(#f3efe4,#191b1e)", AutoHideMenuBar: true})
		d.window.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
			u, err := url.Parse(e.URL)
			if err != nil {
				e.PreventDefault()
				return
			}
			current, _ := url.Parse(d.window.Page().URL())
			if u.Scheme == current.Scheme && u.Host == current.Host {
				return
			}
			e.PreventDefault()
			if e.UserInitiated && (u.Scheme == "http" || u.Scheme == "https") {
				go mygo.Shell.OpenExternal(e.URL)
			}
		})
		d.window.Page().OnRenderProcessGone(func(string) { go d.reconnect() })
		d.window.Page().OnDidFailLoad(func(*mygo.LoadError) { d.window.Page().LoadURL("/") })
		menu := mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu},
			{Label: text("宿主", "Host"), Submenu: []*mygo.MenuItem{
				{Label: text("重新连接", "Reconnect"), Accelerator: "CmdOrCtrl+R", Click: func(*mygo.MenuItem, *mygo.Window) { go d.reconnect() }},
				{Label: text("宿主设置…", "Host settings…"), Click: func(*mygo.MenuItem, *mygo.Window) { d.openSettings() }},
				{Label: text("打开数据目录", "Open data directory"), Click: func(*mygo.MenuItem, *mygo.Window) { go d.openDataDirectory() }},
				mygo.Separator(),
				{Label: text("停止宿主并退出", "Stop host and quit"), Accelerator: "CmdOrCtrl+Q", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
			}},
			{Role: mygo.RoleEditMenu},
			{Label: text("窗口", "Window"), Submenu: []*mygo.MenuItem{{Role: mygo.RoleToggleFullScreen}, {Role: mygo.RoleMinimize}}},
		})
		mygo.App.SetMenu(menu)
		tray, err := mygo.NewTray(mygo.TrayOptions{ID: "bidbrawl", Icon: server.AppIcon(), ToolTip: "BidBrawl",
			Menu: mygo.NewMenu([]*mygo.MenuItem{
				{Label: text("打开 BidBrawl", "Open BidBrawl"), Click: func(*mygo.MenuItem, *mygo.Window) { d.show() }},
				{Label: text("宿主设置…", "Host settings…"), Click: func(*mygo.MenuItem, *mygo.Window) { d.openSettings() }},
				mygo.Separator(),
				{Label: text("停止宿主并退出", "Stop host and quit"), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
			})})
		if err == nil {
			tray.OnClick(d.show)
		}
		d.window.OnClose(func(e *mygo.CloseEvent) {
			if d.approved.Load() {
				return
			}
			e.PreventDefault()
			if d.quitting.Load() {
				return
			}
			if tray != nil {
				d.window.Hide()
			} else {
				mygo.App.Quit()
			}
		})
	})
	err = mygo.App.Run()
	d.op.Lock()
	defer d.op.Unlock()
	return errors.Join(err, d.stopLocked())
}

func (d *Desktop) show() {
	if d.window != nil {
		d.window.Show()
		d.window.Restore()
		d.window.Focus()
	}
}

// Start is called only by the embedded startup page. Host APIs and credentials
// remain in the game service, rather than being bound to the renderer.
func (d *Desktop) Start() error {
	d.op.Lock()
	defer d.op.Unlock()
	if d.loadError != nil {
		return d.loadError
	}
	if d.service != nil {
		d.window.Page().LoadURL(d.service.LaunchURL())
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	done := make(chan error, 1)
	d.done = done
	ready := make(chan *host.Service, 1)
	options := d.options
	var started atomic.Bool
	go func() {
		err := host.Run(ctx, options, func(s *host.Service) { started.Store(true); ready <- s })
		done <- err
		if started.Load() && ctx.Err() == nil {
			d.op.Lock()
			if d.done == done {
				d.service, d.done, d.cancel = nil, nil, nil
			}
			d.op.Unlock()
			d.window.Page().LoadURL("/")
		}
	}()
	select {
	case s := <-ready:
		d.service = s
		d.window.Page().LoadURL(s.LaunchURL())
		return nil
	case err := <-d.done:
		cancel()
		d.cancel = nil
		d.done = nil
		return err
	}
}

func (d *Desktop) stopLocked() error {
	if d.cancel == nil {
		return nil
	}
	d.cancel()
	err := <-d.done
	d.cancel, d.done, d.service = nil, nil, nil
	return err
}

func (d *Desktop) reconnect() {
	if err := d.Start(); err != nil {
		d.window.Page().LoadURL("/")
	}
	d.show()
}

func (d *Desktop) openSettings() {
	if d.settings != nil {
		d.settings.Show()
		d.settings.Focus()
		return
	}
	d.settings = mygo.NewWindow(mygo.WindowOptions{Title: text("宿主设置", "Host settings"), URL: "/settings.html", Width: 620, Height: 540, MinWidth: 520, MinHeight: 500})
	d.settings.OnClosed(func() { d.settings = nil })
}

func (d *Desktop) Settings() host.Options { d.op.Lock(); defer d.op.Unlock(); return d.options }

func (d *Desktop) ChooseDirectory(ctx context.Context) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: text("数据目录", "Data directory"), Directory: true})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func (d *Desktop) Save(ctx context.Context, options host.Options) error {
	_, port, err := net.SplitHostPort(options.Listen)
	if err != nil {
		return errors.New(text("监听地址格式应为 主机:端口", "Use a host:port listen address"))
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return errors.New(text("端口应为 1–65535", "Port must be between 1 and 65535"))
	}
	for _, path := range []*string{&options.DataDir, &options.GameConfig, &options.ItemsConfig, &options.CardsConfig, &options.HintsConfig} {
		if *path != "" {
			*path, err = filepath.Abs(*path)
			if err != nil {
				return err
			}
		}
	}
	d.op.Lock()
	defer d.op.Unlock()
	if d.service != nil {
		accepted, err := confirmOnPage(mygo.CallerWindow(ctx), "保存设置并重启宿主？", "Save settings and restart the host?", "重启", "Restart")
		if err != nil {
			return err
		}
		if !accepted {
			return errors.New(text("已取消", "Cancelled"))
		}
	}
	data, err := json.MarshalIndent(options, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(d.configPath), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(d.configPath), ".desktop-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	_, err = temporary.Write(data)
	err = errors.Join(err, temporary.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(temporary.Name(), d.configPath); err != nil {
		return err
	}
	if err = d.stopLocked(); err != nil {
		return err
	}
	options.WebUI = true
	d.options = options
	d.loadError = nil
	d.window.Page().LoadURL("/")
	mygo.CallerWindow(ctx).Close()
	d.show()
	return nil
}

func (d *Desktop) Quit()         { mygo.App.Quit() }
func (d *Desktop) OpenSettings() { mygo.RunOnMain(d.openSettings) }
func (d *Desktop) SetLocale(locale string) {
	if locale == "zh-CN" || locale == "en" {
		mygo.App.SetLocale(locale)
	}
}
func (d *Desktop) openDataDirectory() {
	d.op.Lock()
	path, err := host.DatabasePath(d.options.DataDir)
	d.op.Unlock()
	if err == nil {
		_ = mygo.Shell.OpenPath(filepath.Dir(path))
	}
}
