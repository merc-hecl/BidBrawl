package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"bidbrawl/internal/cards"
	"bidbrawl/internal/client"
	"bidbrawl/internal/game"
	"bidbrawl/internal/hints"
	"bidbrawl/internal/server"
)

func TestCLIHelpNeverPrintsEnvironmentCredentials(t *testing.T) {
	for _, name := range []string{"BIDBRAWL_TOKEN", "BIDBRAWL_CREDENTIAL", "BIDBRAWL_OBSERVER_TOKEN"} {
		t.Setenv(name, "private-value-for-"+name)
	}
	for _, command := range []string{"bid", "login", "observer-login"} {
		f, err := os.Create(filepath.Join(t.TempDir(), "help.txt"))
		if err != nil {
			t.Fatal(err)
		}
		previous := os.Stderr
		os.Stderr = f
		err = run(context.Background(), []string{command, "-h"})
		os.Stderr = previous
		_ = f.Close()
		if !errors.Is(err, flag.ErrHelp) {
			t.Fatal("help did not stop before execution", err)
		}
		data, err := os.ReadFile(f.Name())
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("private-value-for-")) {
			t.Fatal("help disclosed an environment credential")
		}
		for _, name := range []string{"-token", "-credential", "-observer-token"} {
			if !bytes.Contains(data, []byte(name)) {
				t.Fatal("help lost auth flag", name)
			}
		}
	}
}

// Exercise the command parser and its real HTTP requests, including stdout JSON.
func cli(t *testing.T, args ...string) map[string]any {
	t.Helper()
	data := cliOutput(t, args...)
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(args[0], err)
	}
	return result
}

func cliOutput(t *testing.T, args ...string) []byte {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "stdout.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	previous := os.Stdout
	os.Stdout = f
	err = run(context.Background(), args)
	os.Stdout = previous
	if err != nil {
		t.Fatal(args[0], err)
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCLIHandbookTopicsAndLanguages(t *testing.T) {
	t.Setenv("BIDBRAWL_URL", "http://127.0.0.1:1")
	for _, tc := range []struct {
		name, want, absent string
		args               []string
	}{
		{name: "default_index", want: "# BidBrawl 游玩手册", absent: "## 卡牌目录"},
		{name: "english_index", args: []string{"-lang", "en"}, want: "# BidBrawl handbook", absent: "## Card catalog"},
		{name: "chinese_topic", args: []string{"-topic", "start"}, want: "# 登录与入席", absent: "## 卡牌目录"},
		{name: "english_topic", args: []string{"-topic", "play", "-lang", "en"}, want: "# Each round", absent: "## Card catalog"},
		{name: "chinese_catalog", args: []string{"-topic", "cards"}, want: "## 卡牌目录", absent: "## Card catalog"},
		{name: "english_catalog", args: []string{"-topic", "cards", "-lang", "en"}, want: "Largest-item appraisal", absent: "巨物鉴定"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := string(cliOutput(t, append([]string{"handbook"}, tc.args...)...))
			if !strings.Contains(output, tc.want) || strings.Contains(output, tc.absent) {
				t.Fatalf("unexpected handbook output: %s", output)
			}
		})
	}
	for _, args := range [][]string{
		{"-lang", "fr"}, {"-topic", "../HANDBOOK"}, {"-live"},
	} {
		if err := run(context.Background(), append([]string{"handbook"}, args...)); err == nil {
			t.Fatalf("invalid handbook options accepted: %v", args)
		}
	}
}

func TestCLICatalogFilters(t *testing.T) {
	cfg := game.DefaultConfig()
	cfg.Catalog = []game.ItemTemplate{
		{ID: "vase", Name: "琉璃瓶 & A", Translations: map[string]string{"en": "Glass Vase & A"}, Category: "tools", Quality: "rare", Width: 2, Height: 1, ValuePercent: 50},
		{ID: "other", Name: "Other", Category: "tools", Quality: "common", Width: 1, Height: 1},
	}
	for _, q := range cfg.Qualities {
		cfg.Catalog = append(cfg.Catalog, game.ItemTemplate{ID: "filler_" + q.ID, Name: "Base template", Category: "tools", Quality: q.ID, Width: 1, Height: 1})
	}
	s := server.New(cfg, cards.Default())
	defer s.Close()
	host := httptest.NewServer(s)
	defer host.Close()
	t.Setenv("BIDBRAWL_URL", host.URL)
	if got := cli(t, "catalog"); got["count"] != float64(len(cfg.Catalog)) {
		t.Fatal("default player name accidentally filtered catalog", got)
	}
	created := cli(t, "create", "-token", s.HostToken, "-players", "2")
	for _, name := range []string{"琉璃瓶 & A", "GLASS VASE & A"} {
		got := cli(t, "catalog", "-room", created["room_id"].(string), "-size", "2x1", "-quality", "rare", "-category", "tools", "-name", name)
		items := got["catalog"].([]any)
		if got["count"] != float64(1) || items[0].(map[string]any)["id"] != "vase" || items[0].(map[string]any)["price"] != float64(3000) {
			t.Fatalf("unexpected CLI query response: %+v", got)
		}
	}
	if err := run(context.Background(), []string{"catalog", "-size", "broken"}); err == nil {
		t.Fatal("invalid size accepted")
	}
}

func TestCLIHandbookLiveCatalog(t *testing.T) {
	geometry := game.DefaultConfig()
	geometry.Box, geometry.Items = game.Dimensions{Width: 13, Height: 9}, game.ItemLimits{MaxWidth: 5, MaxHeight: 2}
	cfg := cards.Config{SchemaVersion: 1, DefaultDeck: []string{}, Cards: []cards.Definition{
		{ID: "audit", Name: "Custom audit", Description: "Custom count", Price: 1, Effect: cards.Effect{Type: "count"}},
	}}
	s := server.New(geometry, cfg)
	host := httptest.NewServer(s)
	defer host.Close()
	t.Setenv("BIDBRAWL_URL", host.URL)
	room := cli(t, "create", "-token", s.HostToken, "-players", "2")["room_id"].(string)
	for _, lang := range []string{"zh-CN", "en"} {
		for _, roomArgs := range [][]string{nil, {"-room", room}} {
			args := append([]string{"handbook", "-topic", "cards", "-lang", lang, "-live"}, roomArgs...)
			output := string(cliOutput(t, args...))
			if !strings.Contains(output, "13") || !strings.Contains(output, "117") || !strings.Contains(output, "Custom audit") || strings.Contains(output, "largest_price") {
				t.Fatalf("live handbook did not use host/room configuration: %s", output)
			}
		}
	}
}

func TestCLIHintConfigurationAndPublicCatalog(t *testing.T) {
	exported := cli(t, "hints", "-defaults", "-url", "http://127.0.0.1:1")
	if len(exported["plugins"].([]any)) != len(hints.Default().Plugins) {
		t.Fatal("missing offline default hints")
	}
	cfg := hints.Config{SchemaVersion: 1, Plugins: []hints.Plugin{
		{ID: "custom_a", Command: []string{"private-program", "private-argument"}},
		{ID: "custom_b", Builtin: "random_quality"},
	}}
	s := server.NewWithHints(game.DefaultConfig(), cards.Default(), cfg)
	host := httptest.NewServer(s)
	defer host.Close()
	room := cli(t, "create", "-url", host.URL, "-token", s.HostToken, "-name", "Hints", "-players", "2")["room_id"].(string)
	for _, args := range [][]string{
		{"hints", "-url", host.URL}, {"hints", "-url", host.URL, "-room", room},
	} {
		catalog := cli(t, args...)
		b, _ := json.Marshal(catalog)
		if strings.Contains(string(b), "private-program") || strings.Contains(string(b), "private-argument") {
			t.Fatal("plugin command leaked")
		}
		ids := catalog["public_hint_plugins"].([]any)
		if len(ids) != 2 || ids[0] != "custom_a" || ids[1] != "custom_b" {
			t.Fatal("wrong plugin catalog", catalog)
		}
	}
}

func TestCLIJoinCardSelection(t *testing.T) {
	s := server.New(game.DefaultConfig(), cards.Default())
	host := httptest.NewServer(s)
	defer host.Close()
	t.Setenv("BIDBRAWL_URL", host.URL)
	t.Setenv("BIDBRAWL_TOKEN", s.HostToken)
	room := cli(t, "create", "-players", "3")["room_id"].(string)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "omitted"},
		{name: "empty", args: []string{"-cards", ""}},
		{name: "selected", args: []string{"-cards", "item_count"}, want: "item_count"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			credential := cli(t, "register", "-name", tc.name)["credential"].(string)
			token := cli(t, "login", "-credential", credential)["token"].(string)
			if tc.want != "" {
				cli(t, "buy", "-token", token, "-card", tc.want)
			}
			args := append([]string{"join", "-room", room, "-token", token}, tc.args...)
			cli(t, args...)
			deck := cli(t, "state", "-room", room, "-token", token)["self"].(map[string]any)["deck"].([]any)
			if tc.want == "" && len(deck) != 0 || tc.want != "" && (len(deck) != 1 || deck[0] != tc.want) {
				t.Fatalf("unexpected carried cards: %v", deck)
			}
		})
	}
}

func TestCLIChosenCardTargetIsSentAndStoredPrivately(t *testing.T) {
	s := server.New(game.DefaultConfig(), cards.Default())
	host := httptest.NewServer(s)
	defer host.Close()
	t.Setenv("BIDBRAWL_URL", host.URL)
	t.Setenv("BIDBRAWL_TOKEN", "")
	room := cli(t, "create", "-token", s.HostToken, "-players", "2")["room_id"].(string)
	credential := cli(t, "register", "-name", "Target buyer")["credential"].(string)
	a := cli(t, "login", "-credential", credential)["token"].(string)
	credential = cli(t, "register", "-name", "Other target player")["credential"].(string)
	b := cli(t, "login", "-credential", credential)["token"].(string)
	cli(t, "buy", "-token", a, "-card", "target_quality_count")
	cli(t, "join", "-room", room, "-token", a, "-cards", "target_quality_count")
	cli(t, "join", "-room", room, "-token", b)
	cli(t, "start", "-room", room, "-token", s.HostToken)
	for _, target := range []string{"", "easter_egg", "unknown"} {
		if err := run(context.Background(), []string{"card", "-room", room, "-token", a, "-round", "1", "-card", "target_quality_count", "-target", target, "-reason", "validate target"}); err == nil {
			t.Fatal("bad target accepted")
		}
	}
	v := cli(t, "card", "-room", room, "-token", a, "-round", "1", "-card", "target_quality_count", "-target", "legendary", "-reason", "count one tier from current clues")
	self := v["self"].(map[string]any)
	hint := self["hints"].([]any)[0].(map[string]any)["data"].(map[string]any)
	if hint["target"] != "legendary" || self["card_cost"] != float64(5000) {
		t.Fatal("target or cost lost")
	}
	full := cli(t, "state", "-room", room, "-token", a)
	brief := cli(t, "state", "-room", room, "-token", a, "-compact")
	fullBytes, _ := json.Marshal(full)
	briefBytes, _ := json.Marshal(brief)
	if len(briefBytes)*3 >= len(fullBytes) {
		t.Fatal("compact output retained the repeated catalog/config bulk", len(fullBytes), len(briefBytes))
	}
	compactObservation(full)
	if !reflect.DeepEqual(full, brief) {
		t.Fatal("compact output changed decision state or intel")
	}
	if err := run(context.Background(), []string{"cards", "-compact"}); err == nil {
		t.Fatal("irrelevant compact flag silently ignored")
	}
	for _, token := range []string{"", b} {
		other := cli(t, "state", "-room", room, "-token", token)
		raw, _ := json.Marshal(other)
		if bytes.Contains(raw, []byte(`"target":"legendary"`)) || bytes.Contains(raw, []byte("count one tier from current clues")) {
			t.Fatal("private target leaked")
		}
	}
	if err := run(context.Background(), []string{"catalog", "-target", "art"}); err == nil {
		t.Fatal("irrelevant target silently ignored")
	}
}

func TestCLIProtocolGameFlow(t *testing.T) {
	cfg := cards.Config{SchemaVersion: 1, DefaultDeck: []string{"audit"}, Cards: []cards.Definition{
		{ID: "audit", Name: "自定义全检", Description: "最大物品品质与价格", Price: 1, Effect: cards.Effect{Type: "reveal", Selector: "largest", Fields: []string{"quality", "price"}}},
	}}
	geometry := game.DefaultConfig()
	geometry.Box, geometry.Items = game.Dimensions{Width: 13, Height: 9}, game.ItemLimits{MaxWidth: 5, MaxHeight: 2}
	g, _ := game.NewWithConfig(2, geometry, cfg)
	s := server.New(g.GameConfig(), g.CardConfig())
	host := httptest.NewServer(s)
	defer host.Close()
	t.Setenv("BIDBRAWL_URL", host.URL)
	t.Setenv("BIDBRAWL_TOKEN", "")
	t.Setenv("BIDBRAWL_CREDENTIAL", "")
	var credentials, tokens [2]string
	for i, name := range []string{"Atlas", "Echo"} {
		credentials[i] = cli(t, "register", "-name", name)["credential"].(string)
		tokens[i] = cli(t, "login", "-credential", credentials[i])["token"].(string)
	}
	t.Setenv("BIDBRAWL_TOKEN", tokens[0])
	room := cli(t, "create", "-name", "CLI game", "-players", "2")["room_id"].(string)
	invitation := cli(t, "invite", "-room", room)["invite_url"].(string)
	actualGeometry := cli(t, "config", "-room", room)
	if actualGeometry["box"].(map[string]any)["width"] != float64(13) || actualGeometry["items"].(map[string]any)["max_height"] != float64(2) {
		t.Fatal("created room did not inherit host geometry")
	}
	actual := cli(t, "cards", "-room", room)
	if actual["cards"].([]any)[0].(map[string]any)["id"] != "audit" || actual["default_deck"].([]any)[0] != "audit" {
		t.Fatal("created room did not inherit host configuration")
	}
	if len(cli(t, "rooms")["rooms"].([]any)) != 1 {
		t.Fatal("created room missing from listing")
	}
	for i, token := range tokens {
		t.Setenv("BIDBRAWL_TOKEN", token)
		if i == 0 {
			cli(t, "buy", "-card", "audit")
			cli(t, "join", "-invite", invitation, "-cards", "audit")
		} else {
			cli(t, "join", "-invite", invitation, "-cards", "")
		}
		if len(cli(t, "state", "-room", room)["self"].(map[string]any)["deck"].([]any)) != 1-i {
			t.Fatal("selected cards or explicit empty deck ignored")
		}
	}
	if err := run(context.Background(), []string{"start", "-room", room}); err == nil {
		t.Fatal("non-owner started room")
	}
	t.Setenv("BIDBRAWL_TOKEN", tokens[0])
	cli(t, "start", "-room", room)
	cli(t, "card", "-reason", "protocol test", "-room", room, "-round", "1", "-card", "audit")
	for i, token := range tokens {
		t.Setenv("BIDBRAWL_TOKEN", token)
		self := cli(t, "state", "-room", room)["self"].(map[string]any)
		if len(self["hints"].([]any)) != 1-i {
			t.Fatal("private information isolation failed")
		}
	}
	for round := 1; round <= game.Rounds; round++ {
		for i, token := range tokens {
			t.Setenv("BIDBRAWL_TOKEN", token)
			amount := 990 + 10*i
			if round == game.Rounds {
				amount = 20 + i
			}
			cli(t, "bid", "-reason", "protocol test", "-room", room, "-round", fmt.Sprint(round), "-amount", fmt.Sprint(amount))
		}
	}
	result := cli(t, "state", "-room", room)
	if result["phase"] != "settled" || len(result["history"].([]any)) != game.Rounds || result["settlement"].(map[string]any)["paid"] != float64(21) {
		t.Fatal("five-round settlement failed")
	}
	cli(t, "logout")
	if err := run(context.Background(), []string{"me"}); err == nil {
		t.Fatal("revoked session accepted")
	}
	t.Setenv("BIDBRAWL_CREDENTIAL", credentials[0])
	t.Setenv("BIDBRAWL_TOKEN", cli(t, "login")["token"].(string))
	if cli(t, "me")["name"] != "Atlas" || len(cli(t, "state", "-room", room)["self"].(map[string]any)["hints"].([]any)) != 1 {
		t.Fatal("relogin failed to resume private seat")
	}
	if len(cli(t, "results")["rooms"].([]any)) != 1 || cli(t, "stats")["games"] != float64(1) {
		t.Fatal("missing CLI results/statistics")
	}
	id := cli(t, "me")["account_id"].(string)
	if cli(t, "agent", "-account", id)["games"] != float64(1) {
		t.Fatal("missing agent metrics")
	}
	observer := cli(t, "observer-login", "-token", s.HostToken)["observer_token"].(string)
	opening := cli(t, "replay", "-room", room, "-frame", "0", "-observer", "-observer-token", observer)
	if opening["settlement"] != nil || opening["self"] != nil || len(opening["decisions"].([]any)) != 0 {
		t.Fatal("opening leaked private/future data")
	}
	full := cli(t, "replay", "-room", room, "-observer", "-observer-token", observer)
	if len(full["frames"].([]any)) != 6 {
		t.Fatal("missing replay frames")
	}
	view := cli(t, "state", "-room", room, "-observer", "-observer-token", observer)
	if len(view["decisions"].([]any)) == 0 || view["self"] != nil {
		t.Fatal("invalid observer view")
	}
	if cli(t, "replay", "-room", room)["decisions"] != nil {
		t.Fatal("public replay leaked reasons")
	}
}

func TestCLIRejectsUnknownCommandAndMissingBidAmount(t *testing.T) {
	if err := run(context.Background(), []string{"unknown"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatal("unknown command accepted", err)
	}
	if err := run(context.Background(), []string{"bid", "-round", "1"}); err == nil || !strings.Contains(err.Error(), "-amount is required") {
		t.Fatal("missing bid accepted", err)
	}
}

// Use a real host process and forcibly stop it: persistence must not depend on
// a graceful shutdown hook, on the working directory, or on the old session.
func TestHostIdentityProcess(t *testing.T) {
	if dir := os.Getenv("BIDBRAWL_TEST_HOST_DIR"); dir != "" {
		if err := run(context.Background(), []string{"host", "-listen", "127.0.0.1:0", "-data-dir", dir}); err != nil {
			t.Fatal(err)
		}
		return
	}
	dir := t.TempDir()
	start := func() (string, string, func()) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHostIdentityProcess$")
		cmd.Env = append(os.Environ(), "BIDBRAWL_TEST_HOST_DIR="+dir)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		if err = cmd.Start(); err != nil {
			cancel()
			t.Fatal(err)
		}
		var once sync.Once
		stop := func() {
			once.Do(func() {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				cancel()
			})
		}
		t.Cleanup(stop)
		scanner := bufio.NewScanner(stdout)
		var address, hostToken, path string
		for range 3 {
			if !scanner.Scan() {
				stop()
				t.Fatal("host exited before emitting its startup information")
			}
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "HTTP: "):
				address = strings.TrimPrefix(line, "HTTP: ")
			case strings.HasPrefix(line, "Host token: "):
				hostToken = strings.TrimPrefix(line, "Host token: ")
			case strings.HasPrefix(line, "Identity database: "):
				path = strings.TrimPrefix(line, "Identity database: ")
			}
		}
		if address == "" || hostToken == "" || path != filepath.Join(dir, "accounts.db") {
			stop()
			t.Fatal("host startup information is incomplete or has wrong identity path")
		}
		return address, hostToken, stop
	}
	address, admin, stop := start()
	t.Setenv("BIDBRAWL_URL", address)
	t.Setenv("BIDBRAWL_TOKEN", "")
	t.Setenv("BIDBRAWL_CREDENTIAL", "")
	registered := cli(t, "register", "-name", "Atlas")
	id := registered["account_id"].(string)
	credential := registered["credential"].(string)
	oldToken := cli(t, "login", "-credential", credential)["token"].(string)
	if own := cli(t, "me", "-token", oldToken); own["account_id"] != id || own["name"] != "Atlas" || own["assets"] != float64(game.DefaultConfig().InitialAssets) {
		t.Fatal("account query omitted identity or initial assets")
	}
	if err := run(context.Background(), []string{"me", "-token", oldToken, "-account", id}); err == nil {
		t.Fatal("me accepted an account selector")
	}
	echoCredential := cli(t, "register", "-name", "Echo")["credential"].(string)
	echoToken := cli(t, "login", "-credential", echoCredential)["token"].(string)
	roomID := cli(t, "create", "-token", oldToken, "-name", "Durable game", "-players", "2")["room_id"].(string)
	cli(t, "buy", "-token", oldToken, "-card", "item_count")
	cli(t, "join", "-room", roomID, "-token", oldToken, "-cards", "item_count")
	cli(t, "join", "-room", roomID, "-token", echoToken, "-cards", "")
	cli(t, "start", "-room", roomID, "-token", oldToken)
	cli(t, "card", "-reason", "protocol test", "-room", roomID, "-token", oldToken, "-round", "1", "-card", "item_count")
	privateBefore := cli(t, "bid", "-reason", "protocol test", "-room", roomID, "-token", oldToken, "-round", "1", "-amount", "77777")
	publicBefore := cli(t, "state", "-room", roomID, "-token", "")
	equal := func(a, b map[string]any) bool {
		for _, v := range []map[string]any{a, b} {
			if self, ok := v["self"].(map[string]any); ok {
				delete(self, "activities")
			}
		}
		x, _ := json.Marshal(a)
		y, _ := json.Marshal(b)
		return bytes.Equal(x, y)
	}
	stop()
	address, admin, stop = start()
	t.Setenv("BIDBRAWL_URL", address)
	login := cli(t, "login", "-credential", credential)
	if login["account_id"] != id {
		t.Fatal("forced process restart changed identity")
	}
	at := login["token"].(string)
	if !equal(privateBefore, cli(t, "state", "-room", roomID, "-token", at)) || !equal(publicBefore, cli(t, "state", "-room", roomID, "-token", "")) {
		t.Fatal("forced process restart changed public/private ongoing game state")
	}
	if err := run(context.Background(), []string{"bid", "-reason", "protocol test", "-room", roomID, "-token", at, "-round", "1", "-amount", "1"}); err == nil {
		t.Fatal("forced restart forgot an acknowledged bid")
	}
	if err := run(context.Background(), []string{"me", "-token", oldToken}); err == nil {
		t.Fatal("old session accepted after process restart")
	}
	if err := run(context.Background(), []string{"register", "-name", "Atlas"}); err == nil {
		t.Fatal("registered name became available after process restart")
	}
	if len(cli(t, "accounts", "-token", admin)["accounts"].([]any)) != 2 {
		t.Fatal("host account listing did not restore identity")
	}
	bt := cli(t, "login", "-credential", echoCredential)["token"].(string)
	cli(t, "bid", "-reason", "protocol test", "-room", roomID, "-token", bt, "-round", "1", "-amount", "60000")
	for round := 2; round <= game.Rounds; round++ {
		for _, token := range []string{at, bt} {
			cli(t, "bid", "-reason", "protocol test", "-room", roomID, "-token", token, "-round", fmt.Sprint(round), "-amount", "100")
		}
	}
	settledBefore := cli(t, "state", "-room", roomID, "-token", "")
	selfID := privateBefore["self"].(map[string]any)["id"].(string)
	cashChanges := settledBefore["settlement"].(map[string]any)["cash_changes"].(map[string]any)
	expectedAssets := float64(game.DefaultConfig().InitialAssets-game.DefaultConfig().TicketFee) - 1200 + cashChanges[selfID].(float64)
	ownBefore := cli(t, "me", "-token", at)
	if ownBefore["assets"] != expectedAssets {
		t.Fatal("account query returned stale assets after settlement")
	}
	if !equal(ownBefore["stats"].(map[string]any), cli(t, "agent", "-account", id, "-token", "")) {
		t.Fatal("me statistics differ from player profile")
	}
	board := cli(t, "leaderboard", "-token", "")
	rows := board["rankings"].([]any)
	if board["sort"] != "profit" || board["games"] != float64(1) || len(rows) != 2 || rows[0].(map[string]any)["rank"] != float64(1) {
		t.Fatal("public leaderboard query failed")
	}
	results := func() map[string]any {
		t.Helper()
		var out map[string]any
		if err := client.New(os.Getenv("BIDBRAWL_URL"), "").Request(context.Background(), "GET", "/api/results", nil, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	resultsBefore := results()
	if len(resultsBefore["rooms"].([]any)) != 1 {
		t.Fatal("settled process game missing from statistics")
	}
	rotated := cli(t, "rotate-credential", "-token", admin, "-account", id)
	if rotated["account_id"] != id {
		t.Fatal("CLI credential rotation changed identity")
	}
	stop()
	address, _, stop = start()
	t.Setenv("BIDBRAWL_URL", address)
	if err := run(context.Background(), []string{"login", "-credential", credential}); err == nil {
		t.Fatal("old credential accepted after process restart")
	}
	finalLogin := cli(t, "login", "-credential", rotated["credential"].(string))
	if finalLogin["account_id"] != id {
		t.Fatal("rotated identity did not survive forced process restart")
	}
	if own := cli(t, "me", "-token", finalLogin["token"].(string)); !equal(ownBefore, own) {
		t.Fatal("account query lost assets or statistics after process restart")
	}
	if !equal(settledBefore, cli(t, "state", "-room", roomID, "-token", "")) || !equal(resultsBefore, results()) {
		t.Fatal("forced process restart changed replay, tied winner or statistical history")
	}
	stop()
}

func TestIdentityPathDoesNotDependOnWorkingDirectory(t *testing.T) {
	first, err := identityDBPath("")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	second, err := identityDBPath("")
	if err != nil || second != first || !filepath.IsAbs(second) {
		t.Fatal("default identity path depends on working directory")
	}
	custom, err := identityDBPath("data")
	if err != nil || !filepath.IsAbs(custom) || filepath.Base(custom) != "accounts.db" {
		t.Fatal("custom identity path is not absolute")
	}
}
