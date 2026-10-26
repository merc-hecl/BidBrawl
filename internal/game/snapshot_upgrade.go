package game

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrFutureSnapshot = errors.New("game snapshot requires a newer application")

// UpgradeSnapshot converts the first released format once, before room loading.
// It never generates items, charges accounts, consumes cards or settles a game.
func UpgradeSnapshot(data []byte, now time.Time) ([]byte, error) {
	s, err := decodeSnapshot(data)
	if err != nil {
		return nil, err
	}
	if s.SchemaVersion == SnapshotSchemaVersion {
		return data, nil
	}
	if s.SchemaVersion != 10 {
		return nil, errors.New("unsupported pre-release game snapshot schema")
	}
	s.SchemaVersion = SnapshotSchemaVersion
	if s.Phase == "bidding" {
		s.RoundDeadline = now.UTC().Add(RoundTimeout)
	}
	upgraded, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if _, err := Restore(upgraded); err != nil {
		return nil, err
	}
	return upgraded, nil
}
