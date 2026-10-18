package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/client"
	"bidbrawl/internal/game"
	"bidbrawl/internal/hints"
	"bidbrawl/internal/manual"
	"bidbrawl/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if e := run(ctx, os.Args[1:]); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return
		}
		_ = json.NewEncoder(os.Stderr).Encode(map[string]string{"error": e.Error()})
		os.Exit(1)
	}
}
func printJSON(v any) error {
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
func parseFlags(f *flag.FlagSet, args []string) error {
	var messages bytes.Buffer
	f.SetOutput(&messages)
	e := f.Parse(args)
	if errors.Is(e, flag.ErrHelp) {
		fmt.Fprint(os.Stderr, messages.String())
	}
	return e
}
func run(ctx context.Context, args []string) error {
	return runWithBrowser(ctx, args, openBrowser)
}
func runWithBrowser(ctx context.Context, args []string, launchBrowser func(string) error) error {
	if len(args) == 0 {
		fmt.Println("BidBrawl — 外部 Agent 驱动的暗拍\n命令: host | ui | register | accounts | rotate-credential | login | logout | me | rooms | create | invite | join | start | state | watch | results | stats | leaderboard | agent | replay | observer-login | config | cards | hints | card | bid | withdraw | inventory | buy | sell | loadout | relief | catalog | handbook\n用法: bidbrawl <命令> -h")
		return nil
	}
	command := args[0]
	switch command {
	case "host", "ui", "register", "accounts", "rotate-credential", "login", "logout", "me", "rooms", "create", "invite", "join", "start", "state", "watch", "results", "stats", "leaderboard", "agent", "replay", "observer-login", "config", "cards", "hints", "card", "bid", "handbook", "inventory", "buy", "sell", "loadout", "relief", "catalog", "withdraw":
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	if command == "host" {
		listen := f.String("listen", "127.0.0.1:8787", "listen address; use 0.0.0.0:8787 for LAN access")
		configPath := f.String("cards-config", "", "skill-card JSON configuration path; uses built-in defaults if omitted")
		itemsConfigPath := f.String("items-config", "", "物品目录 JSON 路径，省略使用内置目录")
		gameConfigPath := f.String("game-config", "", "game JSON configuration path: box and item dimensions, quality prices and weights, initial assets; uses built-in defaults if omitted")
		hintsPath := f.String("hints-config", "", "public-hint JSON configuration path; uses built-in local-fact hints if omitted")
		dataDir := f.String("data-dir", os.Getenv("BIDBRAWL_DATA_DIR"), "account, room and game data directory; defaults to BidBrawl in the user configuration directory")
		webUI := f.Bool("webui", false, "enable WebUI and open a browser automatically; defaults to API/SSE only")
		if e := parseFlags(f, args[1:]); e != nil {
			return e
		}
		cfg := cards.Default()
		if *configPath != "" {
			loaded, err := cards.Load(*configPath)
			if err != nil {
				return err
			}
			cfg = loaded
		}
		geometry := game.DefaultConfig()
		if *gameConfigPath != "" {
			loaded, err := game.LoadConfig(*gameConfigPath)
			if err != nil {
				return err
			}
			geometry = loaded
		}
		hintConfig := hints.Default()
		if *hintsPath != "" {
			loaded, err := hints.Load(*hintsPath)
			if err != nil {
				return err
			}
			hintConfig = loaded
		}
		databasePath, e := identityDBPath(*dataDir)
		if e != nil {
			return e
		}
		if *itemsConfigPath != "" {
			items, err := game.LoadCatalog(*itemsConfigPath)
			if err != nil {
				return err
			}
			geometry.Catalog = items
			if err := geometry.Validate(); err != nil {
				return err
			}
		}
		s, e := server.OpenWithHints(geometry, cfg, hintConfig, databasePath)
		if e != nil {
			return e
		}
		defer s.Close()
		if *webUI {
			s.EnableWebUI()
		}
		ln, e := net.Listen("tcp", *listen)
		if e != nil {
			return e
		}
		address := browserAddress(ln.Addr())
		fmt.Printf("HTTP: %s\n", address)
		fmt.Printf("Host token: %s\n", s.HostToken)
		if databasePath != "" {
			fmt.Printf("Identity database: %s\n", databasePath)
		}
		h := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
		if *webUI {
			launchURL := observerLaunchURL(address, "", s.NewObserverLaunchCode())
			go func() {
				if err := launchBrowser(launchURL); err != nil {
					fmt.Fprintln(os.Stderr, "Could not open WebUI automatically; retry with bidbrawl ui -url <host-address> -token <host-token>:", err)
				}
			}()
		}
		hostCtx, hostCancel := context.WithCancel(ctx)
		defer hostCancel()
		stopped := make(chan error, 1)
		go func() {
			<-hostCtx.Done()
			s.CloseEvents()
			shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := h.Shutdown(shutdown)
			if err != nil {
				_ = h.Close()
			}
			stopped <- err
		}()
		e = h.Serve(ln)
		hostCancel()
		shutdownErr := <-stopped // Drain handlers before closing the identity store.
		if errors.Is(e, http.ErrServerClosed) {
			return shutdownErr
		}
		return errors.Join(e, shutdownErr)
	}
	defaultURL := os.Getenv("BIDBRAWL_URL")
	if defaultURL == "" {
		defaultURL = "http://127.0.0.1:8787"
	}
	url := f.String("url", defaultURL, "宿主 HTTP 地址")
	token := f.String("token", os.Getenv("BIDBRAWL_TOKEN"), "登录会话/管理员 Token，推荐用 BIDBRAWL_TOKEN")
	credential := f.String("credential", os.Getenv("BIDBRAWL_CREDENTIAL"), "游戏登录凭据，推荐用 BIDBRAWL_CREDENTIAL")
	room := f.String("room", "", "房间 ID；state / replay / 行动必填，watch 省略则订阅全部房间")
	invitation := f.String("invite", "", "join：房主提供的邀请链接")
	players := f.Int("players", 4, "创建房间的玩家数 (2–7)")
	name := f.String("name", "Agent", "玩家名称；catalog：按中文或译名搜索物品（支持部分名称）")
	size := f.String("size", "", "catalog：物品尺寸，宽x高，单位为格，如 2x2；不自动旋转")
	quality := f.String("quality", "", "catalog：品质 ID 或名称，如 rare；可与尺寸、名称、类别组合")
	accountID := f.String("account", "", "agent / rotate-credential：永久账号 ID，必填")
	deck := f.String("cards", "", "携带的卡 ID，最多 5 张不同卡，用逗号分隔；省略或空字符串表示不带卡")
	specialty := f.String("specialty", "analysis", "鉴定专长 ID，用 catalog 查看22种专长及轮次效果")
	category := f.String("category", "", "专长允许的子品类；catalog：筛选品类 ID；region 可选 northwest/northeast/southwest/southeast")
	ticket := f.Int64("ticket", 5000, "建房门票金额；开局扣除，退出不退，可设0")
	quantity := f.Int("quantity", 1, "购买卡牌数量，1–99")
	itemIDs := f.String("items", "", "出售的仓库物品 ID，逗号分隔")
	sellAll := f.Bool("all", false, "出售全部仓库藏品")
	round := f.Int("round", 0, "用卡 / 报价的当前轮次，必填，从 state 查看")
	card := f.String("card", "", "技能卡 ID")
	amount := f.Int64("amount", 0, "整数报价")
	reason := f.String("reason", "", "报价 / 用卡的私有决策理由，必填，1–2000 字符")
	defaults := f.Bool("defaults", false, "config / cards / hints 命令：离线导出内置默认配置")
	live := f.Bool("live", false, "handbook -topic cards：读取宿主或指定房间的实际配置")
	topic := f.String("topic", "", "handbook：start / play / rules / cards / reconnect / results；省略显示目录")
	lang := f.String("lang", "zh-CN", "handbook：zh-CN / en；默认中文")
	events := f.String("event", "", "watch：逗号分隔的事件名；省略接收当前房间全部事件")
	once := f.Bool("once", false, "watch：收到首个匹配事件后退出")
	waitTimeout := f.Duration("timeout", 0, "watch：总等待期限，例如 2m；0 表示持续订阅")
	observer := f.Bool("observer", false, "state / replay / watch：使用人类只读观察权限")
	observerToken := f.String("observer-token", os.Getenv("BIDBRAWL_OBSERVER_TOKEN"), "独立只读观察凭据；不得分发给参赛 Agent")
	sortKey := f.String("sort", "profit", "stats / leaderboard：profit / rate / average / max_profit / max_loss")
	frame := f.Int("frame", 0, "replay：0 为开局，之后为已揭晓轮次，省略时输出完整复盘与所有帧")
	if e := parseFlags(f, args[1:]); e != nil {
		return e
	}
	explicit := map[string]bool{}
	f.Visit(func(v *flag.Flag) { explicit[v.Name] = true })
	if command == "me" && explicit["account"] {
		return errors.New("me only queries the logged-in account; -account is not supported")
	}
	if *observer && command != "state" && command != "replay" && command != "watch" {
		return errors.New("-observer is only used by state, replay and watch")
	}
	if explicit["frame"] && command != "replay" {
		return errors.New("-frame is only used by replay")
	}
	if *invitation != "" {
		if command != "join" {
			return errors.New("-invite is only used by join")
		}
		inv, err := neturl.Parse(*invitation)
		if err != nil || inv.Scheme != "http" && inv.Scheme != "https" || inv.Host == "" || inv.User != nil || inv.Path != "/invite" || inv.Query().Get("room") == "" {
			return errors.New("invalid invitation link")
		}
		if explicit["room"] && *room != inv.Query().Get("room") || explicit["url"] && strings.TrimRight(*url, "/") != inv.Scheme+"://"+inv.Host {
			return errors.New("invitation conflicts with -room or -url")
		}
		*url, *room = inv.Scheme+"://"+inv.Host, inv.Query().Get("room")
	}
	if *room == "" {
		switch command {
		case "join", "invite", "start", "state", "replay", "card", "loadout", "withdraw":
			return errors.New("-room is required (join also accepts -invite)")
		}
	}
	if command == "cards" && *defaults {
		return printJSON(cards.Default())
	}
	if command == "hints" && *defaults {
		return printJSON(hints.Default())
	}
	if command == "config" && *defaults {
		return printJSON(game.DefaultConfig())
	}
	var handbookText string
	if command == "handbook" {
		var err error
		handbookText, err = manual.Read(*topic, *lang)
		if err != nil {
			return err
		}
		if *live && *topic != "cards" {
			return errors.New("-live requires handbook -topic cards")
		}
		if !*live {
			fmt.Print(handbookText)
			if *token != "" {
				reportCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				local := client.New(*url, *token)
				var result any
				err := local.Request(reportCtx, "POST", client.RoomPath("/api/activity", *room), map[string]any{"command": "handbook", "topic": *topic, "lang": *lang}, &result)
				cancel()
				if err != nil {
					fmt.Fprintln(os.Stderr, "Handbook activity was not recorded:", err)
				}
			}
			if *topic == "cards" {
				fmt.Print(manual.Catalog(cards.Default(), game.DefaultConfig(), *lang))
			}
			return nil
		}
	}
	c := client.New(*url, *token)
	c.Command = command
	if *observer {
		if *observerToken == "" {
			return errors.New("-observer-token or BIDBRAWL_OBSERVER_TOKEN is required")
		}
		c = client.New(*url, *observerToken)
	}
	if command == "ui" {
		var launch struct {
			Code string `json:"code"`
		}
		if err := c.Request(ctx, "POST", "/api/observer/launch", nil, &launch); err != nil {
			return err
		}
		if err := launchBrowser(observerLaunchURL(*url, *room, launch.Code)); err != nil {
			return err
		}
		return printJSON(map[string]bool{"opened": true})
	}
	if command == "watch" {
		if *waitTimeout < 0 {
			return errors.New("-timeout cannot be negative")
		}
		selected := map[string]bool{}
		if *events != "" {
			for _, name := range strings.Split(*events, ",") {
				name = strings.TrimSpace(name)
				if !client.KnownEvent(name) || name == "observer_changed" && !*observer {
					return fmt.Errorf("unknown event: %s", name)
				}
				selected[name] = true
			}
		}
		watchCtx := ctx
		if *waitTimeout > 0 {
			var cancel context.CancelFunc
			watchCtx, cancel = context.WithTimeout(ctx, *waitTimeout)
			defer cancel()
		}
		encoder := json.NewEncoder(os.Stdout)
		watch := c.Watch
		if *observer {
			watch = c.WatchObserver
		}
		err := watch(watchCtx, *room, func(event client.Event) error {
			if event.Name == "ready" && !*observer && *token != "" {
				reportCtx, cancel := context.WithTimeout(watchCtx, 2*time.Second)
				var result any
				reportErr := c.Request(reportCtx, "POST", client.RoomPath("/api/activity", *room), map[string]any{"command": "watch"}, &result)
				cancel()
				if reportErr != nil {
					fmt.Fprintln(os.Stderr, "Watch activity was not recorded:", reportErr)
				}
			}
			matched := len(selected) == 0 || selected[event.Name]
			// Always expose ready so a consumer can resync after reconnect.
			if matched || event.Name == "ready" {
				if err := encoder.Encode(event); err != nil {
					return err
				}
			}
			if *once && matched {
				return client.ErrWatchComplete
			}
			return nil
		})
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return nil
		}
		return err
	}
	if command == "cards" || command == "handbook" || command == "config" || command == "hints" {
		if *room == "" {
			var configuration struct {
				Game        game.Config  `json:"game"`
				Cards       cards.Config `json:"cards"`
				HintPlugins []string     `json:"public_hint_plugins"`
			}
			if err := c.Request(ctx, "GET", "/api/config", nil, &configuration); err != nil {
				return err
			}
			if command == "config" {
				return printJSON(configuration.Game)
			}
			if command == "hints" {
				return printJSON(map[string]any{"public_hint_plugins": configuration.HintPlugins})
			}
			if command == "cards" {
				return printJSON(configuration.Cards)
			}
			fmt.Print(handbookText, manual.Catalog(configuration.Cards, configuration.Game, *lang))
			return nil
		}
		observation, err := c.Observe(ctx, *room)
		if err != nil {
			return err
		}
		if command == "config" {
			return printJSON(observation.Rules.GameConfig)
		}
		if command == "hints" {
			return printJSON(map[string]any{"public_hint_plugins": observation.Rules.PublicHintPlugins})
		}
		cfg := cards.Config{SchemaVersion: observation.Rules.CardsSchemaVersion, DefaultDeck: observation.Rules.DefaultDeck, Cards: observation.Rules.Cards}
		if command == "handbook" {
			fmt.Print(handbookText, manual.Catalog(cfg, observation.Rules.GameConfig, *lang))
			return nil
		}
		return printJSON(cfg)
	}
	method, path := "POST", ""
	var input any
	switch command {
	case "catalog":
		query := neturl.Values{}
		for key, value := range map[string]string{"room": *room, "size": *size, "quality": *quality, "category": *category} {
			if value != "" {
				query.Set(key, value)
			}
		}
		if explicit["name"] {
			query.Set("name", *name)
		}
		method, path = "GET", "/api/catalog?"+query.Encode()
	case "withdraw":
		path = client.RoomPath("/api/withdraw", *room)
		input = map[string]any{"round": *round, "reason": *reason}
	case "buy":
		path = "/api/buy"
		input = map[string]any{"card": *card, "quantity": *quantity}
	case "sell":
		path = "/api/sell"
		ids := []string{}
		if *itemIDs != "" {
			for _, id := range strings.Split(*itemIDs, ",") {
				ids = append(ids, strings.TrimSpace(id))
			}
		}
		input = map[string]any{"ids": ids, "all": *sellAll}
	case "relief":
		path = "/api/relief"
	case "observer-login":
		path = "/api/observer/login"
	case "stats", "leaderboard":
		method, path = "GET", "/api/"+command+"?sort="+neturl.QueryEscape(*sortKey)
	case "agent":
		if *accountID == "" {
			return errors.New("-account is required")
		}
		method, path = "GET", "/api/agents/"+neturl.PathEscape(*accountID)
	case "replay":
		method, path = "GET", client.RoomPath("/api/replay", *room)
		if *observer {
			path = client.RoomPath("/api/observer/replay", *room)
		}
		if explicit["frame"] {
			path += fmt.Sprintf("&frame=%d", *frame)
		}
	case "register":
		path = "/api/register"
		input = map[string]any{"name": *name}
	case "login":
		path = "/api/login"
		input = map[string]any{"credential": *credential}
	case "rotate-credential":
		if *accountID == "" {
			return errors.New("-account is required")
		}
		path = "/api/accounts/" + neturl.PathEscape(*accountID) + "/credential"
	case "logout":
		path = "/api/logout"
	case "me", "rooms", "accounts", "results", "inventory":
		method, path = "GET", "/api/"+command
	case "create":
		path = "/api/create"
		input = map[string]any{"name": *name, "players": *players}
		if explicit["ticket"] {
			input.(map[string]any)["ticket_fee"] = *ticket
		}
	case "invite":
		path = client.RoomPath("/api/invite", *room)
	case "join", "loadout":
		path = client.RoomPath("/api/"+command, *room)
		cards := []string{}
		if *deck != "" {
			for _, id := range strings.Split(*deck, ",") {
				cards = append(cards, strings.TrimSpace(id))
			}
		}
		input = map[string]any{"cards": cards, "specialty": *specialty, "category": *category}
	case "state":
		method, path = "GET", client.RoomPath("/api/state", *room)
		if *observer {
			path = client.RoomPath("/api/observer/state", *room)
		}
	case "start":
		path = client.RoomPath("/api/start", *room)
	case "card":
		if err := game.ValidateReason(*reason); err != nil {
			return err
		}
		path = client.RoomPath("/api/card", *room)
		input = map[string]any{"round": *round, "card": *card, "reason": *reason}
	case "bid":
		if !explicit["amount"] {
			return errors.New("-amount is required and must be positive; use withdraw to exit")
		}
		if *room == "" {
			return errors.New("-room is required")
		}
		if err := game.ValidateReason(*reason); err != nil {
			return err
		}
		path = client.RoomPath("/api/bid", *room)
		input = map[string]any{"round": *round, "amount": *amount, "reason": *reason}
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
	var out map[string]any
	if e := c.Request(ctx, method, path, input, &out); e != nil {
		return e
	}
	if invitePath, ok := out["invite_path"].(string); ok {
		out["invite_url"] = strings.TrimRight(*url, "/") + invitePath
		delete(out, "invite_path")
	}
	return printJSON(out)
}

func identityDBPath(dataDir string) (string, error) {
	if dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("find identity directory: %w", err)
		}
		dataDir = filepath.Join(base, "BidBrawl")
	}
	return filepath.Abs(filepath.Join(dataDir, "accounts.db"))
}
