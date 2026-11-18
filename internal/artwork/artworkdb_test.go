package artwork_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"unicode/utf16"

	"github.com/BrandonKowalski/cw/internal/artwork"
)

type rec struct {
	magic    string
	off      int
	children []*rec
}

func u16(b []byte, o int) uint16 { return binary.LittleEndian.Uint16(b[o:]) }
func u32(b []byte, o int) uint32 { return binary.LittleEndian.Uint32(b[o:]) }
func u64(b []byte, o int) uint64 { return binary.LittleEndian.Uint64(b[o:]) }

func parseRec(b []byte, off int) (*rec, int, error) {
	if off+12 > len(b) {
		return nil, 0, fmt.Errorf("record at %d out of bounds", off)
	}
	r := &rec{magic: string(b[off : off+4]), off: off}
	hl := int(u32(b, off+4))
	switch r.magic {
	case "mhli", "mhla", "mhlf":
		n := int(u32(b, off+8))
		p := off + hl
		for i := 0; i < n; i++ {
			c, end, err := parseRec(b, p)
			if err != nil {
				return nil, 0, err
			}
			r.children = append(r.children, c)
			p = end
		}
		return r, p, nil
	case "mhfd", "mhsd", "mhii", "mhni", "mhod", "mhif":
		total := int(u32(b, off+8))
		if total < hl || off+total > len(b) {
			return nil, 0, fmt.Errorf("%s at %d bad total %d", r.magic, off, total)
		}
		if r.magic == "mhif" || (r.magic == "mhod" && u16(b, off+12) == 3) {
			return r, off + total, nil
		}
		p := off + hl
		for p < off+total {
			c, end, err := parseRec(b, p)
			if err != nil {
				return nil, 0, err
			}
			r.children = append(r.children, c)
			p = end
		}
		if p != off+total {
			return nil, 0, fmt.Errorf("%s at %d children overrun", r.magic, off)
		}
		return r, p, nil
	}
	return nil, 0, fmt.Errorf("unknown record %q at %d", r.magic, off)
}

func parseDB(t *testing.T, b []byte) *rec {
	t.Helper()
	r, end, err := parseRec(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	if end != len(b) || r.magic != "mhfd" {
		t.Fatalf("root %s end %d len %d", r.magic, end, len(b))
	}
	return r
}

func mhodString(t *testing.T, b []byte, m *rec) string {
	t.Helper()
	n := int(u32(b, m.off+0x18))
	raw := b[m.off+0x24 : m.off+0x24+n]
	s := make([]uint16, n/2)
	for i := range s {
		s[i] = u16(raw, i*2)
	}
	return string(utf16.Decode(s))
}

func thumbs(fill1028, fill1029 byte) map[uint32][]byte {
	return map[uint32][]byte{
		1028: bytes.Repeat([]byte{fill1028}, 20000),
		1029: bytes.Repeat([]byte{fill1029}, 80000),
	}
}

func readFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWriteAllLayout(t *testing.T) {
	m := t.TempDir()
	if err := os.MkdirAll(filepath.Join(m, "iPod_Control", "Artwork"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m, "iPod_Control/Artwork/F1015_1.ithmb"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	th := map[uint32][]byte{1028: make([]byte, 20000), 1029: make([]byte, 80000)}
	err := artwork.WriteAll(m, []artwork.Entry{{ImageID: 100, SongDBID: 5, Thumbs: th}, {ImageID: 101, SongDBID: 6, Thumbs: th}})
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(m, "iPod_Control/Artwork/F1029_1.ithmb"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() != 160000 {
		t.Fatal(st.Size())
	}
	if _, err := os.Stat(filepath.Join(m, "iPod_Control/Artwork/F1015_1.ithmb")); !os.IsNotExist(err) {
		t.Fatal("stale ithmb kept")
	}
	db := readFile(t, filepath.Join(m, "iPod_Control/Artwork/ArtworkDB"))
	if string(db[:4]) != "mhfd" || binary.LittleEndian.Uint32(db[8:12]) != uint32(len(db)) {
		t.Fatal("header")
	}
}

func findMHII(t *testing.T, db []byte) []*rec {
	t.Helper()
	root := parseDB(t, db)
	mhli := root.children[0].children[0]
	if mhli.magic != "mhli" {
		t.Fatal(mhli.magic)
	}
	return mhli.children
}

func mhniFor(t *testing.T, db []byte, mhii *rec, format uint32) *rec {
	t.Helper()
	for _, mhod := range mhii.children {
		for _, mhni := range mhod.children {
			if u32(db, mhni.off+0x10) == format {
				return mhni
			}
		}
	}
	t.Fatalf("no mhni for %d", format)
	return nil
}

func TestWriteAllMHIIOffsets(t *testing.T) {
	m := t.TempDir()
	th := thumbs(0, 0)
	if err := artwork.WriteAll(m, []artwork.Entry{{ImageID: 100, SongDBID: 5, Thumbs: th}, {ImageID: 101, SongDBID: 6, Thumbs: th}}); err != nil {
		t.Fatal(err)
	}
	db := readFile(t, filepath.Join(m, "iPod_Control/Artwork/ArtworkDB"))
	mhiis := findMHII(t, db)
	if len(mhiis) != 2 {
		t.Fatal(len(mhiis))
	}
	second := mhiis[1]
	if got := u64(db, second.off+0x14); got != 6 {
		t.Fatal("song dbid", got)
	}
	if got := u32(db, mhniFor(t, db, second, 1029).off+0x14); got != 80000 {
		t.Fatal("offset", got)
	}
	if got := u32(db, mhniFor(t, db, second, 1028).off+0x14); got != 20000 {
		t.Fatal("offset", got)
	}
}

func TestWriteAllLibgpodFields(t *testing.T) {
	m := t.TempDir()
	es := []artwork.Entry{
		{ImageID: 100, SongDBID: 0x1122334455667788, Thumbs: thumbs(1, 2), SourceSize: 54321},
		{ImageID: 101, SongDBID: 9, Thumbs: thumbs(3, 4), SourceSize: 7},
		{ImageID: 102, SongDBID: 10, Thumbs: thumbs(1, 2), SourceSize: 54321},
	}
	if err := artwork.WriteAll(m, es); err != nil {
		t.Fatal(err)
	}
	db := readFile(t, filepath.Join(m, "iPod_Control/Artwork/ArtworkDB"))
	root := parseDB(t, db)
	checks := []struct {
		name      string
		got, want uint64
	}{
		{"mhfd header_len", uint64(u32(db, 4)), 0x84},
		{"mhfd unknown1", uint64(u32(db, 0x0c)), 0},
		{"mhfd unknown2", uint64(u32(db, 0x10)), 2},
		{"mhfd num_children", uint64(u32(db, 0x14)), 3},
		{"mhfd next_id", uint64(u32(db, 0x1c)), 102},
		{"mhfd unknown_flag1", uint64(db[0x30]), 2},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d want %d", c.name, c.got, c.want)
		}
	}
	if len(root.children) != 3 {
		t.Fatal(len(root.children))
	}
	lists := []string{"mhli", "mhla", "mhlf"}
	for i, sd := range root.children {
		if sd.magic != "mhsd" || u32(db, sd.off+4) != 0x60 || u16(db, sd.off+0x0c) != uint16(i+1) {
			t.Errorf("mhsd %d wrong", i)
		}
		if len(sd.children) != 1 || sd.children[0].magic != lists[i] || u32(db, sd.children[0].off+4) != 0x5c {
			t.Errorf("mhsd %d list wrong", i)
		}
	}
	if n := len(root.children[1].children[0].children); n != 0 || u32(db, root.children[1].children[0].off+8) != 0 {
		t.Error("mhla not empty")
	}
	mhlf := root.children[2].children[0]
	if u32(db, mhlf.off+8) != 2 || len(mhlf.children) != 2 {
		t.Fatal("mhlf count")
	}
	gotFiles := map[uint32]uint32{}
	for i, f := range mhlf.children {
		if f.magic != "mhif" || u32(db, f.off+4) != 0x7c || u32(db, f.off+8) != 0x7c || u32(db, f.off+0x0c) != 0 {
			t.Errorf("mhif %d header", i)
		}
		gotFiles[u32(db, f.off+0x10)] = u32(db, f.off+0x14)
	}
	if u32(db, mhlf.children[0].off+0x10) != 1029 {
		t.Error("mhif order not libgpod fallback order")
	}
	if gotFiles[1028] != 20000 || gotFiles[1029] != 80000 {
		t.Error(gotFiles)
	}
	mhli := root.children[0].children[0]
	if u32(db, mhli.off+8) != 3 || len(mhli.children) != 3 {
		t.Fatal("mhli count")
	}
	for i, mhii := range mhli.children {
		e := es[i]
		o := mhii.off
		if u32(db, o+4) != 0x98 || u32(db, o+0x0c) != 2 || u32(db, o+0x10) != e.ImageID || u64(db, o+0x14) != e.SongDBID {
			t.Errorf("mhii %d ids", i)
		}
		for _, z := range []int{0x1c, 0x20, 0x24, 0x28, 0x2c} {
			if u32(db, o+z) != 0 {
				t.Errorf("mhii %d field %#x nonzero", i, z)
			}
		}
		if u32(db, o+0x30) != e.SourceSize+1 {
			t.Errorf("mhii %d orig_img_size %d", i, u32(db, o+0x30))
		}
		if len(mhii.children) != 2 {
			t.Fatal("mhii children")
		}
		var order []uint32
		for _, mhod := range mhii.children {
			mo := mhod.off
			if mhod.magic != "mhod" || u32(db, mo+4) != 0x18 || u16(db, mo+0x0c) != 2 || len(mhod.children) != 1 {
				t.Fatalf("mhod type 2 wrong")
			}
			mhni := mhod.children[0]
			no := mhni.off
			fid := u32(db, no+0x10)
			order = append(order, fid)
			w := map[uint32]uint32{1028: 100, 1029: 200}[fid]
			if mhni.magic != "mhni" || u32(db, no+4) != 0x4c || u32(db, no+0x0c) != 1 {
				t.Errorf("mhni header")
			}
			if u32(db, no+0x14) != uint32(i)*w*w*2 || u32(db, no+0x18) != w*w*2 {
				t.Errorf("mhni %d offset/size %d %d", fid, u32(db, no+0x14), u32(db, no+0x18))
			}
			if u16(db, no+0x1c) != 0 || u16(db, no+0x1e) != 0 || u16(db, no+0x20) != uint16(w) || u16(db, no+0x22) != uint16(w) {
				t.Errorf("mhni %d padding/dims", fid)
			}
			s := mhni.children[0]
			so := s.off
			if s.magic != "mhod" || u32(db, so+4) != 0x18 || u32(db, so+8) != 64 || u16(db, so+0x0c) != 3 || db[so+0x0e] != 0 || db[so+0x0f] != 0 || u32(db, so+0x18) != 28 || db[so+0x1c] != 2 {
				t.Errorf("mhod type 3 header wrong")
			}
			if got := mhodString(t, db, s); got != fmt.Sprintf(":F%d_1.ithmb", fid) {
				t.Error(got)
			}
		}
		if order[0] != 1029 || order[1] != 1028 {
			t.Errorf("mhni order %v", order)
		}
	}
	f1028 := readFile(t, filepath.Join(m, "iPod_Control/Artwork/F1028_1.ithmb"))
	f1029 := readFile(t, filepath.Join(m, "iPod_Control/Artwork/F1029_1.ithmb"))
	if len(f1028) != 60000 || len(f1029) != 240000 {
		t.Fatal(len(f1028), len(f1029))
	}
	if f1028[0] != 1 || f1028[20000] != 3 || f1028[40000] != 1 || f1029[80000] != 4 || f1029[239999] != 2 {
		t.Error("ithmb contents not in entry order")
	}
}

func TestWriteAllRollsOverIthmbFiles(t *testing.T) {
	defer artwork.SetMaxIthmbSize(160000)()
	m := t.TempDir()
	dir := filepath.Join(m, "iPod_Control", "Artwork")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"F1029_9.ithmb", "F1028_1.ithmb", "keep.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var es []artwork.Entry
	for i := 0; i < 5; i++ {
		es = append(es, artwork.Entry{ImageID: uint32(100 + i), SongDBID: uint64(i + 1), Thumbs: thumbs(byte(i), byte(i))})
	}
	if err := artwork.WriteAll(m, es); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	want := []string{"ArtworkDB", "F1028_1.ithmb", "F1029_1.ithmb", "F1029_2.ithmb", "F1029_3.ithmb", "keep.txt"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatal(names)
	}
	sizes := map[string]int{"F1028_1.ithmb": 100000, "F1029_1.ithmb": 160000, "F1029_2.ithmb": 160000, "F1029_3.ithmb": 80000}
	for n, s := range sizes {
		if got := len(readFile(t, filepath.Join(dir, n))); got != s {
			t.Errorf("%s size %d want %d", n, got, s)
		}
	}
	db := readFile(t, filepath.Join(dir, "ArtworkDB"))
	mhiis := findMHII(t, db)
	wantLoc := []struct {
		file string
		off  uint32
	}{{"1", 0}, {"1", 80000}, {"2", 0}, {"2", 80000}, {"3", 0}}
	for i, mhii := range mhiis {
		mhni := mhniFor(t, db, mhii, 1029)
		if got := u32(db, mhni.off+0x14); got != wantLoc[i].off {
			t.Errorf("entry %d offset %d", i, got)
		}
		if got := mhodString(t, db, mhni.children[0]); got != ":F1029_"+wantLoc[i].file+".ithmb" {
			t.Errorf("entry %d file %s", i, got)
		}
	}
}

func TestWriteAllEmpty(t *testing.T) {
	m := t.TempDir()
	dir := filepath.Join(m, "iPod_Control", "Artwork")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "F1028_1.ithmb"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := artwork.WriteAll(m, nil); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name() != "ArtworkDB" {
		t.Fatal(ents)
	}
	db := readFile(t, filepath.Join(dir, "ArtworkDB"))
	root := parseDB(t, db)
	if u32(db, 0x1c) != 0 || len(root.children[0].children[0].children) != 0 || len(root.children[2].children[0].children) != 2 {
		t.Fatal("empty db wrong")
	}
}

func TestWriteAllRejectsBadThumbs(t *testing.T) {
	m := t.TempDir()
	if err := artwork.WriteAll(m, []artwork.Entry{{ImageID: 100, SongDBID: 1, Thumbs: map[uint32][]byte{1028: make([]byte, 20000)}}}); err == nil {
		t.Fatal("missing 1029 accepted")
	}
	if err := artwork.WriteAll(m, []artwork.Entry{{ImageID: 100, SongDBID: 1, Thumbs: map[uint32][]byte{1028: make([]byte, 10), 1029: make([]byte, 80000)}}}); err == nil {
		t.Fatal("short 1028 accepted")
	}
}
