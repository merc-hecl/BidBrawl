package game

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestAvatarsAreUniqueAndPersistAcrossRestart(t *testing.T) {
	g, err := New(7)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 7 {
		if _, _, err := g.Join(fmt.Sprintf("Player %d", i), nil); err != nil {
			t.Fatal(err)
		}
	}
	lobby, _ := g.Observe("")
	var before View
	if err := json.Unmarshal(lobby, &before); err != nil {
		t.Fatal(err)
	}
	for _, p := range before.Players {
		if p.Avatar != nil {
			t.Fatal("avatar assigned before start")
		}
	}
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	data, _ := g.Snapshot()
	var snapshot gameSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, p := range snapshot.Players {
		if p.Avatar == nil || *p.Avatar < 0 || *p.Avatar >= 7 || seen[*p.Avatar] {
			t.Fatal("invalid or duplicate avatar")
		}
		seen[*p.Avatar] = true
	}
	restored, err := Restore(data)
	if err != nil {
		t.Fatal(err)
	}
	public, _ := g.Observe("")
	after, _ := restored.Observe("")
	if string(public) != string(after) {
		t.Fatal("restart changed avatar assignment")
	}
	for _, kind := range []string{"missing", "out of range", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			var invalid gameSnapshot
			_ = json.Unmarshal(data, &invalid)
			switch kind {
			case "missing":
				invalid.Players[0].Avatar = nil
			case "out of range":
				n := 7
				invalid.Players[0].Avatar = &n
			case "duplicate":
				invalid.Players[0].Avatar = invalid.Players[1].Avatar
			}
			encoded, _ := json.Marshal(invalid)
			if _, err := Restore(encoded); err == nil {
				t.Fatal("accepted corrupt avatar assignment")
			}
		})
	}
}
