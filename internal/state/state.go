package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BrandonKowalski/cw/internal/fsutil"
	"github.com/BrandonKowalski/cw/internal/library"
)

type Track struct {
	DBID          uint64       `json:"dbid"`
	DevicePath    string       `json:"device_path"`
	SourceSize    int64        `json:"source_size"`
	SourceMTime   int64        `json:"source_mtime"`
	TagFP         string       `json:"tag_fp"`
	AudioFP       string       `json:"audio_fp"`
	EncodeVersion string       `json:"encode_version"`
	Tags          library.Tags `json:"tags"`
	DurationMS    int          `json:"duration_ms"`
	SampleRate    int          `json:"sample_rate"`
	BitrateKbps   int          `json:"bitrate_kbps"`
	FileSize      int64        `json:"file_size"`
	DateAdded     int64        `json:"date_added"`
	ArtID         string       `json:"art_id"`
	ArtSize       uint32       `json:"art_size"`
}

type State struct {
	Version   int               `json:"version"`
	LastSync  int64             `json:"last_sync"`
	DBPending bool              `json:"db_pending"`
	Tracks    map[string]*Track `json:"tracks"`
}

func statePath(mount string) string {
	return filepath.Join(mount, "iPod_Control", "cw", "state.json")
}

func Load(mount string) (*State, error) {
	b, err := os.ReadFile(statePath(mount))
	if os.IsNotExist(err) {
		return &State{Version: 1, Tracks: map[string]*Track{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	s := &State{}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if s.Tracks == nil {
		s.Tracks = map[string]*Track{}
	}
	return s, nil
}

func Save(mount string, s *State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := fsutil.WriteAtomic(statePath(mount), b); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}
