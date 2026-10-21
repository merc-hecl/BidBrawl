package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	neturl "net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/client"
	"bidbrawl/internal/game"
	"bidbrawl/internal/hints"
	"bidbrawl/internal/host"
	"bidbrawl/internal/manual"
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
		var options host.Options
		options.Flags(f)
		f.BoolVar(&options.WebUI, "webui", false, "enable WebUI and open a browser automatically; defaults to API/SSE only")
		if err := parseFlags(f, args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("unexpected host argument: %s", f.Arg(0))
		}
		return host.Run(ctx, options, func(service *host.Service) {
			fmt.Printf("HTTP: %s\nHost token: %s\nIdentity database: %s\n", service.Address, service.HostToken(), service.DatabasePath)
			if options.WebUI {
				if err := launchBrowser(service.LaunchURL()); err != nil {
					fmt.Fprintln(os.Stderr, "Could not open WebUI automatically; retry with bidbrawl ui:", err)
				}
			}
		})
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
	specialty := f.String("specialty", "analysis", "鉴定专长 ID，用 catalog 查看专长及轮次效果")
	category := f.String("category", "", "专长允许的子品类；catalog：筛选品类 ID；region 可选 northwest/northeast/southwest/southeast")
	ticket := f.Int64("ticket", 5000, "建房门票金额；开局扣除，退出不退，可设0")
	quantity := f.Int("quantity", 1, "购买卡牌数量，1–99")
	itemIDs := f.String("items", "", "出售的仓库物品 ID，逗号分隔")
	sellAll := f.Bool("all", false, "出售全部仓库藏品")
	round := f.Int("round", 0, "用卡 / 报价的当前轮次，必填，从 state 查看")
	card := f.String("card", "", "技能卡 ID")
	cardTarget := f.String("target", "", "card：卡牌要求的品类、品质、面积、象限或已观察物品 ID；以 cards 中 target 字段为准")
	compact := f.Bool("compact", false, "state / card / bid / withdraw：省略重复目录、配置副本和行动流水，保留全部决策状态与情报")
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
	// Flag help prints DefValue. Preserve environment-based authentication without
	// copying credentials into help text, screenshots or command transcripts.
	for _, name := range []string{"token", "credential", "observer-token"} {
		f.Lookup(name).DefValue = ""
	}
	sortKey := f.String("sort", "profit", "stats / leaderboard：profit / rate / average / max_profit / max_loss")
	frame := f.Int("frame", 0, "replay：0 为开局，之后为已揭晓轮次，省略时输出完整复盘与所有帧")
	if e := parseFlags(f, args[1:]); e != nil {
		return e
	}
	explicit := map[string]bool{}
	f.Visit(func(v *flag.Flag) { explicit[v.Name] = true })
	if explicit["compact"] && command != "state" && command != "card" && command != "bid" && command != "withdraw" {
		return errors.New("-compact is only used by state, card, bid or withdraw")
	}
	if explicit["target"] && command != "card" {
		return errors.New("-target is only used by card")
	}
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
		input = map[string]any{"round": *round, "card": *card, "target": *cardTarget, "reason": *reason}
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
	if *compact {
		compactObservation(out)
	}
	return printJSON(out)
}

// This is an output projection of the authorized response, never an estimator.
// Round state, bids, observations, decisions and settlement remain unchanged.
func compactObservation(out map[string]any) {
	if rules, ok := out["rules"].(map[string]any); ok {
		delete(rules, "catalog")
		delete(rules, "game_config")
	}
	delete(out, "activities")
	if self, ok := out["self"].(map[string]any); ok {
		delete(self, "activities")
	}
}

func identityDBPath(dataDir string) (string, error) { return host.DatabasePath(dataDir) }
