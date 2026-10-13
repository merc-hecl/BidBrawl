package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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

// Exercise the command parser and its real HTTP requests, including stdout JSON.
func cli(t *testing.T, args ...string) map[string]any {
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
	var result map[string]any
	if err = json.Unmarshal(data, &result); err != nil {
		t.Fatal(args[0], err)
	}
	return result
}

func TestCLIHintConfigurationAndPublicCatalog(t *testing.T) {
	exported := cli(t, "hints", "-defaults", "-url", "http://127.0.0.1:1")
	if len(exported["plugins"].([]any)) != 5 {
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

func TestCLIProtocolGameFlow(t *testing.T) {
	cfg := cards.Config{SchemaVersion: 1, DefaultDeck: []string{"audit"}, Cards: []cards.Definition{
		{ID: "audit", Name: "自定义全检", Description: "最大物品品质与价格", Effect: cards.Effect{Type: "reveal", Selector: "largest", Fields: []string{"quality", "price"}}},
	}}
	geometry := game.Config{SchemaVersion: 1, Box: game.Dimensions{Width: 13, Height: 9}, Items: game.ItemLimits{MaxWidth: 5, MaxHeight: 2}}
	g, _ := game.NewWithConfig(2, geometry, cfg)
	s := server.New(g.GameConfig(), g.CardConfig())
	host := httptest.NewServer(s)
	defer host.Close()
	t.Setenv("BIDBRAWL_URL", host.URL)
	t.Setenv("BIDBRAWL_TOKEN", "")
	t.Setenv("BIDBRAWL_CREDENTIAL", "")
	var credentials, tokens [2]string
	for i, name := range []string{"Atlas", "Echo"} {
		credentials[i] = cli(t, "register", "-token", s.HostToken, "-name", name)["credential"].(string)
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
			cli(t, "join", "-invite", invitation)
		} else {
			cli(t, "join", "-invite", invitation, "-cards", "")
		}
		if len(cli(t, "state", "-room", room)["self"].(map[string]any)["deck"].([]any)) != 1-i {
			t.Fatal("default deck or explicit empty deck ignored")
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
			amount := 900 + 100*i
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
	if len(full["frames"].([]any)) != 7 {
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

func TestRemovedCommandsAndMissingBid(t *testing.T) {
	for _, command := range []string{"mcp", "tools", "demo"} {
		for _, args := range [][]string{{command}, {command, "-h"}} {
			err := run(context.Background(), args)
			if err == nil || !strings.Contains(err.Error(), "unknown command") {
				t.Fatal("removed command accepted", args, err)
			}
		}
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
			case strings.HasPrefix(line, "房主 Token: "):
				hostToken = strings.TrimPrefix(line, "房主 Token: ")
			case strings.HasPrefix(line, "身份数据库: "):
				path = strings.TrimPrefix(line, "身份数据库: ")
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
	registered := cli(t, "register", "-token", admin, "-name", "Atlas")
	id := registered["account_id"].(string)
	credential := registered["credential"].(string)
	oldToken := cli(t, "login", "-credential", credential)["token"].(string)
	echoCredential := cli(t, "register", "-token", admin, "-name", "Echo")["credential"].(string)
	echoToken := cli(t, "login", "-credential", echoCredential)["token"].(string)
	roomID := cli(t, "create", "-token", oldToken, "-name", "Durable game", "-players", "2")["room_id"].(string)
	cli(t, "join", "-room", roomID, "-token", oldToken, "-cards", "item_count")
	cli(t, "join", "-room", roomID, "-token", echoToken, "-cards", "")
	cli(t, "start", "-room", roomID, "-token", oldToken)
	cli(t, "card", "-reason", "protocol test", "-room", roomID, "-token", oldToken, "-round", "1", "-card", "item_count")
	privateBefore := cli(t, "bid", "-reason", "protocol test", "-room", roomID, "-token", oldToken, "-round", "1", "-amount", "777777")
	publicBefore := cli(t, "state", "-room", roomID, "-token", "")
	equal := func(a, b map[string]any) bool {
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
	if err := run(context.Background(), []string{"register", "-token", admin, "-name", "Atlas"}); err == nil {
		t.Fatal("registered name became available after process restart")
	}
	if len(cli(t, "accounts", "-token", admin)["accounts"].([]any)) != 2 {
		t.Fatal("host account listing did not restore identity")
	}
	bt := cli(t, "login", "-credential", echoCredential)["token"].(string)
	cli(t, "bid", "-reason", "protocol test", "-room", roomID, "-token", bt, "-round", "1", "-amount", "600000")
	for round := 2; round <= game.Rounds; round++ {
		for _, token := range []string{at, bt} {
			cli(t, "bid", "-reason", "protocol test", "-room", roomID, "-token", token, "-round", fmt.Sprint(round), "-amount", "100")
		}
	}
	settledBefore := cli(t, "state", "-room", roomID, "-token", "")
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
	if cli(t, "login", "-credential", rotated["credential"].(string))["account_id"] != id {
		t.Fatal("rotated identity did not survive forced process restart")
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
