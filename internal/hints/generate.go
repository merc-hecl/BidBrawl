package hints

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os/exec"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type Item struct {
	ID          string `json:"id"`
	Category    string `json:"category,omitempty"`
	Quality     string `json:"quality"`
	TopLeft     Point  `json:"top_left"`
	BottomRight Point  `json:"bottom_right"`
}

// Request excludes prices, identities, players, bids, skills and credentials.
type Request struct {
	Categories      map[string]string `json:"categories,omitempty"`
	ProtocolVersion int               `json:"protocol_version"`
	Round           int               `json:"round"`
	BoxWidth        int               `json:"box_width"`
	BoxHeight       int               `json:"box_height"`
	Items           []Item            `json:"items"`
}

type Text struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// Clue combines plain display text with an optional approved public fact.
type Clue struct {
	Text
	Translations map[string]Text `json:"translations,omitempty"`
	Fact         *Fact           `json:"fact,omitempty"`
}

func (c Clue) Validate() error {
	if c.Fact != nil {
		if err := c.Fact.Validate(); err != nil {
			return err
		}
	}
	valid := func(t Text) bool {
		return validText(t.Title, 80) && validText(t.Message, 1000)
	}
	if !valid(c.Text) || len(c.Translations) > 2 {
		return errors.New("hint needs title/message (max 80/1000 characters)")
	}
	for lang, t := range c.Translations {
		if (lang != "zh-CN" && lang != "en") || !valid(t) {
			return errors.New("invalid hint translation")
		}
	}
	return nil
}

func validText(s string, max int) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max && strings.IndexFunc(s, unicode.IsControl) == -1
}

func ParseClue(r io.Reader) (Clue, error) {
	var c Clue
	if err := decode(r, &c, 16<<10); err != nil {
		return c, err
	}
	return c, c.Validate()
}

func Random(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err)
	}
	return int(v.Int64())
}

func (p Plugin) Generate(ctx context.Context, r Request) (Clue, error) {
	if p.Builtin != "" {
		return builtin(p.Builtin, r)
	}
	input, err := json.Marshal(r)
	if err != nil {
		return Clue{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.Command[0], p.Command[1:]...)
	cmd.Stdin = bytes.NewReader(input)
	output := &limitedOutput{}
	cmd.Stdout = output
	// Do not echo plugin stderr: it could contain the full private box.
	cmd.WaitDelay = 100 * time.Millisecond
	if err := cmd.Run(); err != nil {
		return Clue{}, fmt.Errorf("hint plugin %s failed", p.ID)
	}
	clue, err := ParseClue(bytes.NewReader(output.buffer.Bytes()))
	if err != nil {
		return Clue{}, fmt.Errorf("hint plugin %s returned invalid text", p.ID)
	}
	return clue, nil
}

// Keep the buffer private: embedding it would promote ReadFrom and let io.Copy
// bypass Write's limit when collecting a child process's stdout.
type limitedOutput struct{ buffer bytes.Buffer }

func (b *limitedOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > 16<<10 {
		return 0, errors.New("hint output exceeds 16 KiB")
	}
	return b.buffer.Write(p)
}
