package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"bidbrawl/internal/client"
	"bidbrawl/internal/game"
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
