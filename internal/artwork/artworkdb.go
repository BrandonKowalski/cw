package artwork

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf16"

	"github.com/BrandonKowalski/cw/internal/fsutil"
)

type Entry struct {
	ImageID    uint32
	SongDBID   uint64
	Thumbs     map[uint32][]byte
	SourceSize uint32
}

var maxIthmbSize = 256 * 1000 * 1000

type slot struct {
	file   int
	offset uint32
}

func dbFormats() []Format {
	fs := slices.Clone(Formats)
	slices.Reverse(fs)
	return fs
}

func ithmbName(id uint32, file int) string {
	return fmt.Sprintf("F%d_%d.ithmb", id, file)
}

func WriteAll(mount string, entries []Entry) error {
	dir := filepath.Join(mount, "iPod_Control", "Artwork")
	formats := dbFormats()
	for i, e := range entries {
		for _, f := range formats {
			if n := len(e.Thumbs[f.ID]); n != f.W*f.H*2 {
				return fmt.Errorf("artwork entry %d image %d: format %d thumb is %d bytes, want %d", i, e.ImageID, f.ID, n, f.W*f.H*2)
			}
		}
	}
	slots := make(map[uint32][]slot, len(formats))
	keep := map[string]bool{}
	for _, f := range formats {
		size := f.W * f.H * 2
		file, off := 1, 0
		ss := make([]slot, len(entries))
		for i := range entries {
			if off >= maxIthmbSize {
				file++
				off = 0
			}
			ss[i] = slot{file: file, offset: uint32(off)}
			off += size
		}
		slots[f.ID] = ss
		if err := writeIthmbs(dir, f, entries, ss, keep); err != nil {
			return err
		}
	}
	db := buildDB(entries, formats, slots)
	if err := fsutil.WriteAtomic(filepath.Join(dir, "ArtworkDB"), db); err != nil {
		return fmt.Errorf("write ArtworkDB: %w", err)
	}
	stale, err := filepath.Glob(filepath.Join(dir, "F*.ithmb"))
	if err != nil {
		return fmt.Errorf("list ithmb files: %w", err)
	}
	for _, p := range stale {
		if keep[filepath.Base(p)] {
			continue
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s: %w", p, err)
		}
	}
	return nil
}

func writeIthmbs(dir string, f Format, entries []Entry, ss []slot, keep map[string]bool) error {
	for start := 0; start < len(entries); {
		end := start
		for end < len(entries) && ss[end].file == ss[start].file {
			end++
		}
		buf := make([]byte, 0, (end-start)*f.W*f.H*2)
		for _, e := range entries[start:end] {
			buf = append(buf, e.Thumbs[f.ID]...)
		}
		name := ithmbName(f.ID, ss[start].file)
		if err := fsutil.WriteAtomic(filepath.Join(dir, name), buf); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		keep[name] = true
		start = end
	}
	return nil
}

type rec []byte

func header(magic string, headerLen int) rec {
	r := make(rec, headerLen)
	copy(r, magic)
	binary.LittleEndian.PutUint32(r[4:], uint32(headerLen))
	return r
}

func (r rec) u16(off int, v uint16) { binary.LittleEndian.PutUint16(r[off:], v) }
func (r rec) u32(off int, v uint32) { binary.LittleEndian.PutUint32(r[off:], v) }
func (r rec) u64(off int, v uint64) { binary.LittleEndian.PutUint64(r[off:], v) }

func withChildren(r rec, children ...[]byte) []byte {
	for _, c := range children {
		r = append(r, c...)
	}
	binary.LittleEndian.PutUint32(r[8:], uint32(len(r)))
	return r
}

func list(magic string, children [][]byte) []byte {
	r := header(magic, 0x5c)
	r.u32(8, uint32(len(children)))
	for _, c := range children {
		r = append(r, c...)
	}
	return r
}

func mhsd(index uint16, child []byte) []byte {
	r := header("mhsd", 0x60)
	r.u16(0x0c, index)
	return withChildren(r, child)
}

func mhodFilename(name string) []byte {
	u := utf16.Encode([]rune(name))
	n := len(u) * 2
	pad := (4 - (0x24+n)%4) % 4
	r := header("mhod", 0x24)
	r.u32(4, 0x18)
	r.u16(0x0c, 3)
	r[0x0f] = byte(pad)
	r.u32(0x18, uint32(n))
	r[0x1c] = 2
	for _, c := range u {
		r = binary.LittleEndian.AppendUint16(r, c)
	}
	r = append(r, make([]byte, pad)...)
	r.u32(8, uint32(len(r)))
	return r
}

func mhni(f Format, s slot) []byte {
	r := header("mhni", 0x4c)
	r.u32(0x0c, 1)
	r.u32(0x10, f.ID)
	r.u32(0x14, s.offset)
	r.u32(0x18, uint32(f.W*f.H*2))
	r.u16(0x20, uint16(f.H))
	r.u16(0x22, uint16(f.W))
	return withChildren(r, mhodFilename(":"+ithmbName(f.ID, s.file)))
}

func mhodLocation(child []byte) []byte {
	r := header("mhod", 0x18)
	r.u16(0x0c, 2)
	return withChildren(r, child)
}

func mhii(e Entry, i int, formats []Format, slots map[uint32][]slot) []byte {
	r := header("mhii", 0x98)
	r.u32(0x0c, uint32(len(formats)))
	r.u32(0x10, e.ImageID)
	r.u64(0x14, e.SongDBID)
	r.u32(0x30, e.SourceSize+1)
	var children [][]byte
	for _, f := range formats {
		children = append(children, mhodLocation(mhni(f, slots[f.ID][i])))
	}
	return withChildren(r, children...)
}

func mhif(f Format) []byte {
	r := header("mhif", 0x7c)
	r.u32(8, 0x7c)
	r.u32(0x10, f.ID)
	r.u32(0x14, uint32(f.W*f.H*2))
	return r
}

func buildDB(entries []Entry, formats []Format, slots map[uint32][]slot) []byte {
	var images, files [][]byte
	var nextID uint32
	for i, e := range entries {
		images = append(images, mhii(e, i, formats, slots))
		nextID = max(nextID, e.ImageID)
	}
	for _, f := range formats {
		files = append(files, mhif(f))
	}
	r := header("mhfd", 0x84)
	r.u32(0x10, 2)
	r.u32(0x14, 3)
	r.u32(0x1c, nextID)
	r[0x30] = 2
	return withChildren(r,
		mhsd(1, list("mhli", images)),
		mhsd(2, list("mhla", nil)),
		mhsd(3, list("mhlf", files)),
	)
}
