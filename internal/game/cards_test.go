package game

import (
	"bytes"
	"testing"
)

func TestCardRoundGateIsAtomicAndDurable(t *testing.T) {
	g, _ := New(2)
	_, a, _ := g.Join("A", []string{"quality_counts", "random_shape"})
	_, b, _ := g.Join("B", nil)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	before := mustSnapshot(t, g)
	if err := g.UseCard(a, 1, "quality_counts", "", "seek full value"); err == nil {
		t.Fatal("early audit accepted")
	}
	if !bytes.Equal(before, mustSnapshot(t, g)) {
		t.Fatal("rejected card changed costs, allowance or observations")
	}
	if err := g.UseCard(a, 1, "random_shape", "", "use an unlocked card instead"); err != nil {
		t.Fatal(err)
	}
	for round := 1; round < 5; round++ {
		for _, tok := range []string{a, b} {
			if err := g.Submit(tok, round, 100, "equal offers for gate fixture"); err != nil {
				t.Fatal(err)
			}
		}
	}
	g, _ = Restore(mustSnapshot(t, g))
	public, _ := g.Observe("")
	if err := g.UseCard(a, 5, "quality_counts", "", "value the final offer"); err != nil {
		t.Fatal(err)
	}
	after, _ := g.Observe("")
	if !bytes.Equal(public, after) || len(view(t, g, b).Self.Hints) != 0 {
		t.Fatal("audit leaked to public or peer")
	}
	v := view(t, g, a)
	if v.Self.Hints[1].Data["counts"] == nil || v.Self.CardCost != 12200 {
		t.Fatal("unlocked audit result or cost incorrect")
	}
	restored, err := Restore(mustSnapshot(t, g))
	if err != nil {
		t.Fatal(err)
	}
	x, _ := g.Observe(a)
	y, _ := restored.Observe(a)
	if !bytes.Equal(x, y) {
		t.Fatal("restart changed learned audit")
	}
}

func TestChosenCardTargetIsPrivateAtomicAndSurvivesRestore(t *testing.T) {
	for _, tc := range []struct{ card, target, key string }{
		{"category_shapes", "art", "items"},
		{"target_quality_count", "legendary", "count"},
		{"region_census", "northwest", "counts"},
	} {
		t.Run(tc.card, func(t *testing.T) {
			g, _ := New(2)
			_, a, _ := g.Join("A", []string{tc.card})
			_, b, _ := g.Join("B", nil)
			if err := g.Start(); err != nil {
				t.Fatal(err)
			}
			for g.round < 3 {
				round := g.round
				for _, tok := range []string{a, b} {
					if err := g.Submit(tok, round, 100, "equal offers for target fixture"); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := mustSnapshot(t, g)
			for _, target := range []string{"", "invalid", "easter_egg"} {
				if g.UseCard(a, 3, tc.card, target, "test rejected target") == nil {
					t.Fatal("invalid target accepted", target)
				}
				if !bytes.Equal(before, mustSnapshot(t, g)) {
					t.Fatal("invalid target changed allowance, state or costs")
				}
			}
			public, _ := g.Observe("")
			if err := g.UseCard(a, 3, tc.card, tc.target, "follow my visible observations"); err != nil {
				t.Fatal(err)
			}
			d := view(t, g, a).Self.Hints[0].Data
			if d["target"] != tc.target {
				t.Fatal("chosen target not retained")
			}
			if _, ok := d[tc.key]; !ok {
				t.Fatal("missing targeted result", d)
			}
			after, _ := g.Observe("")
			if !bytes.Equal(public, after) || len(view(t, g, b).Self.Hints) > 0 {
				t.Fatal("target or result leaked")
			}
			restored, err := Restore(mustSnapshot(t, g))
			if err != nil {
				t.Fatal(err)
			}
			x, _ := g.Observe(a)
			y, _ := restored.Observe(a)
			if !bytes.Equal(x, y) {
				t.Fatal("restart changed target or result")
			}
		})
	}
}
