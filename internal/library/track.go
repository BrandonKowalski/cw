package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BrandonKowalski/cw/internal/flac"
)

type Entry struct {
	RelPath string
	Size    int64
	ModTime time.Time
}

type Track struct {
	Entry   Entry
	Tags    Tags
	Stream  flac.StreamInfo
	Cover   []byte
	TagFP   string
	AudioFP string
}

var folderImages = []string{"cover.jpg", "cover.png", "folder.jpg", "folder.png"}

func PickPicture(pics []flac.Picture) []byte {
	for _, p := range pics {
		if p.Type == 3 {
			return p.Data
		}
	}
	if len(pics) > 0 {
		return pics[0].Data
	}
	return nil
}

func folderCover(dir string) []byte {
	for _, n := range folderImages {
		if b, err := os.ReadFile(filepath.Join(dir, n)); err == nil {
			return b
		}
	}
	return nil
}

func AudioFingerprint(path string, s flac.StreamInfo, audioOffset int64) (string, error) {
	if s.MD5 != [16]byte{} {
		return hex.EncodeToString(s.MD5[:]), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Seek(audioOffset, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func tagFingerprint(t Tags, cover []byte) (string, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	cs := sha256.Sum256(cover)
	h.Write(b)
	h.Write(cs[:])
	return hex.EncodeToString(h.Sum(nil)), nil
}

func Load(root string, e Entry) (*Track, error) {
	path := filepath.Join(root, filepath.FromSlash(e.RelPath))
	md, err := flac.ReadFile(path)
	if err != nil {
		return nil, err
	}
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	cover := PickPicture(md.Pictures)
	if cover == nil {
		cover = folderCover(filepath.Dir(path))
	}
	tags := TagsFromVorbis(md.Tags, stem)
	tfp, err := tagFingerprint(tags, cover)
	if err != nil {
		return nil, err
	}
	afp, err := AudioFingerprint(path, md.Stream, md.AudioOffset)
	if err != nil {
		return nil, errors.Join(errors.New("audio fingerprint"), err)
	}
	return &Track{Entry: e, Tags: tags, Stream: md.Stream, Cover: cover, TagFP: tfp, AudioFP: afp}, nil
}
