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
	assigned := map[string]int{}
	checkAvatars := func() {
		t.Helper()
		public, err := g.Observe("")
		if err != nil {
			t.Fatal(err)
		}
		var view View
		if err := json.Unmarshal(public, &view); err != nil {
			t.Fatal(err)
		}
		seen := map[int]bool{}
		for _, p := range view.Players {
			if p.Avatar == nil || *p.Avatar < 0 || *p.Avatar >= 7 || seen[*p.Avatar] {
				t.Fatal("invalid or duplicate avatar")
			}
			if previous, ok := assigned[p.ID]; ok && previous != *p.Avatar {
				t.Fatal("existing player avatar changed")
			}
			assigned[p.ID], seen[*p.Avatar] = *p.Avatar, true
		}
		snapshot, err := g.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		restored, err := Restore(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		after, err := restored.Observe("")
		if err != nil {
			t.Fatal(err)
		}
		if string(public) != string(after) {
			t.Fatal("restart changed avatar assignment")
		}
		g = restored
	}
	for i := range 7 {
		if _, _, err := g.Join(fmt.Sprintf("Player %d", i), nil); err != nil {
			t.Fatal(err)
		}
		checkAvatars()
	}
	lobby, _ := g.Snapshot()
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	checkAvatars()
	data, _ := g.Snapshot()
	for phase, data := range map[string][]byte{"lobby": lobby, "bidding": data} {
		for _, kind := range []string{"missing", "out of range", "duplicate"} {
			t.Run(phase+"/"+kind, func(t *testing.T) {
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
}
