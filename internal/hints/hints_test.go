package hints

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestConfigRejectsAmbiguousOrInvalidPlugins(t *testing.T) {
	for _, mutate := range []func(*Config){
		func(c *Config) { c.SchemaVersion = 2 },
		func(c *Config) { c.Plugins = c.Plugins[:1] },
		func(c *Config) { c.Plugins[1].ID = c.Plugins[0].ID },
		func(c *Config) { c.Plugins[0].Builtin = "unknown" },
		func(c *Config) { c.Plugins[0].Command = []string{"plugin"} },
		func(c *Config) { c.Plugins[0].Builtin = "" },
	} {
		c := Default()
		mutate(&c)
		b, _ := json.Marshal(c)
		if _, err := Parse(bytes.NewReader(b)); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	for _, input := range []string{`{} {}`, `{"schema_version":1,"plugins":[],"unknown":true}`} {
		if _, err := Parse(strings.NewReader(input)); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
}

func TestBuiltinFactsAreAccurateAndLimited(t *testing.T) {
	r := Request{ProtocolVersion: 1, Round: 1, BoxWidth: 5, BoxHeight: 1, Items: []Item{
		{ID: "a", Quality: "common", Price: 20, TopLeft: Point{0, 0}, BottomRight: Point{1, 1}},
		{ID: "b", Quality: "common", Price: 100, TopLeft: Point{1, 0}, BottomRight: Point{2, 1}},
		{ID: "c", Quality: "rare", Price: 600, TopLeft: Point{2, 0}, BottomRight: Point{3, 1}},
		{ID: "d", Quality: "epic", Price: 1000, TopLeft: Point{3, 0}, BottomRight: Point{4, 1}},
		{ID: "e", Quality: "common", Price: 50, TopLeft: Point{4, 0}, BottomRight: Point{5, 1}},
	}}
	for _, p := range Default().Plugins {
		for range 80 {
			c, err := p.Generate(context.Background(), r)
			if err != nil || c.Validate() != nil || c.Fact == nil {
				t.Fatal(c, err)
			}
			f := c.Fact
			var group []Item
			for _, item := range r.Items {
				if item.Quality == f.Quality {
					group = append(group, item)
				}
			}
			switch p.Builtin {
			case "quality_presence":
				if *f.Present != (len(group) > 0) {
					t.Fatal("inaccurate presence", f)
				}
			case "quality_largest_area":
				largest := 0
				for _, item := range group {
					largest = max(largest, (item.BottomRight.X-item.TopLeft.X)*(item.BottomRight.Y-item.TopLeft.Y))
				}
				if *f.Area != largest {
					t.Fatal("inaccurate largest area", f)
				}
			case "quality_count":
				if *f.Count != len(group) {
					t.Fatal("inaccurate count", f)
				}
			case "quality_average":
				var sum int64
				for _, item := range group {
					sum += item.Price
				}
				if len(group)*2 > len(r.Items) || *f.AveragePrice != float64(sum)/float64(len(group)) {
					t.Fatal("inaccurate or overly broad average", f)
				}
			case "random_shape", "random_quality", "random_quality_price":
				found := false
				for _, item := range r.Items {
					if item.ID == f.ItemID {
						found = true
						if f.Quality != "" && f.Quality != item.Quality {
							t.Fatal("inaccurate quality")
						}
						if f.Price != nil && *f.Price != item.Price {
							t.Fatal("inaccurate item price")
						}
						if f.TopLeft != nil && (*f.TopLeft != item.TopLeft || *f.BottomRight != item.BottomRight) {
							t.Fatal("inaccurate shape")
						}
					}
				}
				if !found {
					t.Fatal("unknown revealed item")
				}
			default:
				t.Fatal("unexpected builtin", p.Builtin)
			}
		}
	}
}

func TestBroadPricesAndRemovedHintsAreUnavailable(t *testing.T) {
	r := Request{Items: []Item{{ID: "a", Quality: "common", Price: 60, BottomRight: Point{1, 1}}}}
	for _, kind := range []string{"quality_average", "random_quality_price"} {
		if _, err := builtin(kind, r); !errors.Is(err, ErrNotApplicable) {
			t.Fatal("single item reveals whole value", kind, err)
		}
	}
	r.Items = append(r.Items, Item{ID: "b", Quality: "common", Price: 90, BottomRight: Point{2, 1}})
	if _, err := builtin("quality_average", r); !errors.Is(err, ErrNotApplicable) {
		t.Fatal("whole-box average revealed")
	}
	for _, kind := range []string{"item_count", "total_value", "value_band", "value_mood", "value_spread", "random_dimensions", "random_area", "random_aspect", "random_region", "quality_region_count", "quality_count_compare", "random_quality_compare", "random_price_compare"} {
		cfg := Default()
		cfg.Plugins[0].Builtin = kind
		if cfg.Validate() == nil {
			t.Fatal("removed hint accepted", kind)
		}
		if _, err := builtin(kind, r); err == nil {
			t.Fatal("removed builtin still generates a clue", kind)
		}
	}
	for _, text := range []string{
		`{"title":"x","message":"y","fact":{"type":"item_count","count":5}}`,
		`{"title":"x","message":"y","fact":{"type":"total_value","price":100}}`,
		`{"title":"x","message":"y","fact":{"type":"quality_count","quality":"common","count":2,"price":100}}`,
		`{"title":"x","message":"y","fact":{"type":"quality_presence","quality":"rare","present":true,"count":2}}`,
		`{"title":"x","message":"y","fact":{"type":"quality_largest_area","quality":"rare","area":0}}`,
		`{"title":"x","message":"y","fact":{"type":"random_region","item_id":"a","region":"top_left"}}`,
	} {
		if _, err := ParseClue(strings.NewReader(text)); err == nil {
			t.Fatal("overly broad fact accepted")
		}
	}
}

func TestLargestQualityAreaUsesRectanglesAndPresenceKeepsFalse(t *testing.T) {
	r := Request{Items: []Item{
		{ID: "a", Quality: "rare", BottomRight: Point{2, 3}},
		{ID: "b", Quality: "rare", TopLeft: Point{2, 0}, BottomRight: Point{5, 4}},
		{ID: "c", Quality: "epic", TopLeft: Point{0, 4}, BottomRight: Point{1, 5}},
	}}
	for range 40 {
		c, err := builtin("quality_largest_area", r)
		if err != nil || c.Fact == nil {
			t.Fatal(c, err)
		}
		if *c.Fact.Area != map[string]int{"rare": 12, "epic": 1}[c.Fact.Quality] {
			t.Fatal("incorrect maximum rectangle area", c)
		}
	}
	c, err := ParseClue(strings.NewReader(`{"title":"No legendary items","message":"None present","fact":{"type":"quality_presence","quality":"legendary","present":false}}`))
	if err != nil || c.Fact.Present == nil || *c.Fact.Present {
		t.Fatal("absence must be preserved as an explicit false", c, err)
	}
	data, _ := json.Marshal(c)
	if !bytes.Contains(data, []byte(`"present":false`)) {
		t.Fatal("absence disappeared during serialization")
	}
}

// The test binary serves as a real external plugin process, on every platform.
func TestHintProcess(t *testing.T) {
	if len(os.Args) < 2 || !strings.HasPrefix(os.Args[len(os.Args)-1], "hint-helper:") {
		return
	}
	mode := strings.TrimPrefix(os.Args[len(os.Args)-1], "hint-helper:")
	var req map[string]json.RawMessage
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		os.Exit(2)
	}
	if len(req) != 5 || req["items"] == nil || req["protocol_version"] == nil {
		os.Exit(3)
	}
	switch mode {
	case "slow":
		time.Sleep(5 * time.Second)
	case "failure":
		fmt.Fprint(os.Stderr, "private box contents")
		os.Exit(4)
	case "oversize":
		fmt.Print(strings.Repeat("x", 17<<10))
	case "extra":
		fmt.Print(`{"title":"x","message":"y","price":60}`)
	case "trailing":
		fmt.Print(`{"title":"x","message":"y"} {}`)
	default:
		fmt.Print(`{"title":"自定义印象","message":"也许藏着一些惊喜。","translations":{"en":{"title":"Custom impression","message":"There might be a surprise."}}}`)
	}
	os.Exit(0)
}

func TestExecutableProtocolLimitsAndErrors(t *testing.T) {
	r := Request{ProtocolVersion: 1, Round: 3, BoxWidth: 20, BoxHeight: 30, Items: []Item{{Quality: "common", Price: 60, BottomRight: Point{2, 2}}}}
	for _, mode := range []string{"success", "failure", "oversize", "extra", "trailing", "slow"} {
		t.Run(mode, func(t *testing.T) {
			p := Plugin{ID: "custom", Command: []string{os.Args[0], "-test.run=^TestHintProcess$", "--", "hint-helper:" + mode}}
			started := time.Now()
			c, err := p.Generate(context.Background(), r)
			if mode == "success" {
				if err != nil || c.Title != "自定义印象" {
					t.Fatal(c, err)
				}
			} else if err == nil || strings.Contains(err.Error(), "private box") {
				t.Fatal("unsafe or missing plugin failure", err)
			}
			if mode == "slow" && time.Since(started) > 4*time.Second {
				t.Fatal("timeout not enforced")
			}
		})
	}
}

func TestPluginOutputLimitCannotBeBypassedByCopy(t *testing.T) {
	output := &limitedOutput{}
	_, err := io.Copy(output, strings.NewReader(strings.Repeat("x", 1<<20)))
	if err == nil || output.buffer.Len() > 16<<10 {
		t.Fatal("unbounded plugin output buffer")
	}
}
