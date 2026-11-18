package apply

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BrandonKowalski/cw/internal/artwork"
	"github.com/BrandonKowalski/cw/internal/device"
	"github.com/BrandonKowalski/cw/internal/fsutil"
	"github.com/BrandonKowalski/cw/internal/itunesdb"
	"github.com/BrandonKowalski/cw/internal/state"
)

const (
	firstTrackID   = 52
	firstArtworkID = 100
)

func WriteDatabases(d *device.Device, st *state.State) error {
	keys := make([]string, 0, len(st.Tracks))
	for k := range st.Tracks {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return less(st.Tracks[keys[i]], st.Tracks[keys[j]], keys[i], keys[j])
	})
	tracks := make([]itunesdb.Track, 0, len(keys))
	var arts []artwork.Entry
	nextArt := uint32(firstArtworkID)
	for i, k := range keys {
		s := st.Tracks[k]
		t := itunesdb.Track{
			ID:          uint32(firstTrackID + i),
			DBID:        s.DBID,
			Path:        ":" + strings.ReplaceAll(s.DevicePath, "/", ":"),
			Tags:        s.Tags,
			DurationMS:  s.DurationMS,
			SampleRate:  s.SampleRate,
			BitrateKbps: s.BitrateKbps,
			Size:        s.FileSize,
			DateAdded:   time.Unix(s.DateAdded, 0),
		}
		if s.ArtID != "" {
			thumbs, err := loadThumbs(d.Mount, s.ArtID)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err == nil {
				t.ArtworkID = nextArt
				t.ArtworkSize = s.ArtSize
				arts = append(arts, artwork.Entry{ImageID: nextArt, SongDBID: s.DBID, Thumbs: thumbs, SourceSize: s.ArtSize})
				nextArt++
			}
		}
		tracks = append(tracks, t)
	}
	if err := artwork.WriteAll(d.Mount, arts); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := itunesdb.Write(&buf, tracks, rand.Uint64()); err != nil {
		return fmt.Errorf("build iTunesDB: %w", err)
	}
	if err := fsutil.WriteAtomic(filepath.Join(d.Mount, "iPod_Control", "iTunes", "iTunesDB"), buf.Bytes()); err != nil {
		return fmt.Errorf("write iTunesDB: %w", err)
	}
	return nil
}

func less(a, b *state.Track, ka, kb string) bool {
	if c := strings.Compare(fold(albumArtist(a)), fold(albumArtist(b))); c != 0 {
		return c < 0
	}
	if c := strings.Compare(fold(a.Tags.Album), fold(b.Tags.Album)); c != 0 {
		return c < 0
	}
	if a.Tags.DiscNumber != b.Tags.DiscNumber {
		return a.Tags.DiscNumber < b.Tags.DiscNumber
	}
	if a.Tags.TrackNumber != b.Tags.TrackNumber {
		return a.Tags.TrackNumber < b.Tags.TrackNumber
	}
	if c := strings.Compare(fold(a.Tags.Title), fold(b.Tags.Title)); c != 0 {
		return c < 0
	}
	return ka < kb
}

func albumArtist(t *state.Track) string {
	if t.Tags.SortAlbumArtist != "" {
		return t.Tags.SortAlbumArtist
	}
	return t.Tags.AlbumArtist
}

func fold(s string) string {
	return strings.ToLower(s)
}

func loadThumbs(mount, id string) (map[uint32][]byte, error) {
	out := make(map[uint32][]byte, len(artwork.Formats))
	for _, f := range artwork.Formats {
		b, err := os.ReadFile(artPath(mount, id, f))
		if err != nil {
			return nil, err
		}
		if len(b) != f.W*f.H*2 {
			return nil, fmt.Errorf("art cache %s format %d: %w", id, f.ID, fs.ErrNotExist)
		}
		out[f.ID] = b
	}
	return out, nil
}
