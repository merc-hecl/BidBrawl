// Package hints generates public clues about limited parts of an auction lot.
package hints

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
)

//go:embed default.json
var defaults []byte

type Plugin struct {
	ID      string   `json:"id"`
	Builtin string   `json:"builtin,omitempty"`
	Command []string `json:"command,omitempty"`
}

type Config struct {
	SchemaVersion int      `json:"schema_version"`
	Plugins       []Plugin `json:"plugins"`
}

func Default() Config {
	c, err := Parse(bytes.NewReader(defaults))
	if err != nil {
		panic(err)
	}
	return c
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	cfg, err := Parse(f)
	if err != nil {
		return Config{}, fmt.Errorf("hints config %s: %w", path, err)
	}
	return cfg, nil
}

func Parse(r io.Reader) (Config, error) {
	var c Config
	if err := decode(r, &c, 1<<20); err != nil {
		return c, err
	}
	return c, c.Validate()
}

func decode(r io.Reader, v any, limit int64) error {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return err
	}
	if int64(len(b)) > limit {
		return errors.New("hint JSON exceeds size limit")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("expected exactly one hint JSON object")
	}
	return nil
}

var validID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func (c Config) Validate() error {
	if c.SchemaVersion != 1 || len(c.Plugins) < 2 || len(c.Plugins) > 64 {
		return errors.New("hints require schema_version=1 and 2–64 plugins")
	}
	seen := map[string]bool{}
	nonPrice := 0
	for _, p := range c.Plugins {
		if !validID.MatchString(p.ID) || seen[p.ID] {
			return fmt.Errorf("invalid or duplicate hint plugin: %q", p.ID)
		}
		seen[p.ID] = true
		if (p.Builtin == "") == (len(p.Command) == 0) {
			return fmt.Errorf("hint %s needs exactly one of builtin or command", p.ID)
		}
		if p.Builtin != "" {
			if _, ok := builtins[p.Builtin]; !ok {
				return fmt.Errorf("unknown hint builtin: %s", p.Builtin)
			}
		}
		for _, arg := range p.Command {
			if arg == "" || len(arg) > 4096 {
				return fmt.Errorf("hint %s has invalid command argument", p.ID)
			}
		}
		if p.Builtin != "quality_average" && p.Builtin != "random_quality_price" {
			nonPrice++
		}
		if len(p.Command) > 32 {
			return errors.New("hint command accepts at most 32 arguments")
		}
	}
	if nonPrice == 0 {
		return errors.New("hints require at least one non-price plugin")
	}
	return nil
}

func (c Config) Clone() Config {
	out := Config{SchemaVersion: c.SchemaVersion, Plugins: append([]Plugin{}, c.Plugins...)}
	for i := range out.Plugins {
		out.Plugins[i].Command = append([]string(nil), c.Plugins[i].Command...)
	}
	return out
}

// IDs is the public catalog; local executable paths and arguments stay private.
func (c Config) IDs() []string {
	ids := []string{}
	for _, p := range c.Plugins {
		ids = append(ids, p.ID)
	}
	return ids
}
