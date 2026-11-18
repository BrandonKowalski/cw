package itunesdb_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"reflect"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/BrandonKowalski/cw/internal/itunesdb"
	"github.com/BrandonKowalski/cw/internal/itunesdb/itunesdbtest"
	"github.com/BrandonKowalski/cw/internal/library"
)

func readDB(t *testing.T, b []byte) []itunesdbtest.Track {
	t.Helper()
	got, err := itunesdbtest.Read(b)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func masterPlaylistIDs(t *testing.T, b []byte) []uint32 {
	t.Helper()
	ids, err := itunesdbtest.MasterPlaylistIDs(b)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func sampleTracks() []itunesdb.Track {
	return []itunesdb.Track{
		{ID: 1, DBID: 0xAABBCCDD11223344, Path: ":iPod_Control:Music:F01:ABCD.m4a",
			Tags:       library.Tags{Title: "Hourglass", Artist: "Sleep Theory", AlbumArtist: "Sleep Theory", Album: "Afterglow", TrackNumber: 2, DiscNumber: 1},
			DurationMS: 164940, SampleRate: 44100, BitrateKbps: 900, Size: 18000000, ArtworkID: 100, ArtworkSize: 100000},
		{ID: 2, DBID: 2, Path: ":iPod_Control:Music:F02:WXYZ.m4a", Tags: library.Tags{Title: "Ünïcödé _Slut!_"}, DurationMS: 1, SampleRate: 48000},
	}
}

func write(t *testing.T, tr []itunesdb.Track) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := itunesdb.Write(&b, tr, 99); err != nil {
		t.Fatal(err)
	}
	if b.Len() < 244 {
		t.Fatalf("short output %d", b.Len())
	}
	return b.Bytes()
}

func TestWriteRoundTrip(t *testing.T) {
	tr := sampleTracks()
	var b bytes.Buffer
	if err := itunesdb.Write(&b, tr, 99); err != nil {
		t.Fatal(err)
	}
	got := readDB(t, b.Bytes())
	if len(got) != 2 || got[0].Title != "Hourglass" || got[0].DBID != 0xAABBCCDD11223344 || got[0].Path != tr[0].Path || got[0].MHIILink != 100 || got[0].SampleRate != 44100 {
		t.Fatalf("%+v", got)
	}
	if got[0].Album != "Afterglow" || got[0].Artist != "Sleep Theory" || got[0].AlbumArtist != "Sleep Theory" || got[0].TrackNumber != 2 || got[0].DiscNumber != 1 || got[0].DurationMS != 164940 || got[0].Size != 18000000 || got[0].ID != 1 {
		t.Fatalf("%+v", got[0])
	}
	if got[1].Title != "Ünïcödé _Slut!_" || got[1].MHIILink != 0 {
		t.Fatalf("%+v", got[1])
	}
	if !reflect.DeepEqual(masterPlaylistIDs(t, b.Bytes()), []uint32{1, 2}) {
		t.Fatal()
	}
}

type node struct {
	magic string
	off   int
	end   int
	depth int
}

var listMagic = map[string]bool{"mhlt": true, "mhlp": true, "mhla": true, "mhli": true}

func walk(t *testing.T, b []byte, start, end, depth int, out *[]node) {
	t.Helper()
	for off := start; off < end; {
		if off+12 > end {
			t.Fatalf("truncated chunk at %d (parent end %d)", off, end)
		}
		magic := string(b[off : off+4])
		hl := int(binary.LittleEndian.Uint32(b[off+4:]))
		tl := int(binary.LittleEndian.Uint32(b[off+8:]))
		if hl < 12 || off+hl > end {
			t.Fatalf("%s at %d: header length %d exceeds parent end %d", magic, off, hl, end)
		}
		if listMagic[magic] {
			*out = append(*out, node{magic, off, off + hl, depth})
			off += hl
			continue
		}
		if tl < hl || off+tl > end {
			t.Fatalf("%s at %d: total length %d exceeds parent end %d", magic, off, tl, end)
		}
		*out = append(*out, node{magic, off, off + tl, depth})
		if magic != "mhod" {
			walk(t, b, off+hl, off+tl, depth+1, out)
		}
		off += tl
	}
}

func chunks(t *testing.T, b []byte) []node {
	t.Helper()
	var out []node
	walk(t, b, 0, len(b), 0, &out)
	return out
}

func le32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }
func le16(b []byte, off int) uint16 { return binary.LittleEndian.Uint16(b[off:]) }
func le64(b []byte, off int) uint64 { return binary.LittleEndian.Uint64(b[off:]) }

func TestWriteHeaderMagicAndLengths(t *testing.T) {
	b := write(t, sampleTracks())
	if len(b) < 244 {
		t.Fatalf("short output %d", len(b))
	}
	if string(b[:4]) != "mhbd" {
		t.Fatalf("magic %q", b[:4])
	}
	if le32(b, 4) != 244 {
		t.Fatalf("header length %d", le32(b, 4))
	}
	if int(le32(b, 8)) != len(b) {
		t.Fatalf("total %d len %d", le32(b, 8), len(b))
	}
	if le32(b, 0x10) != 0x30 {
		t.Fatalf("version %#x", le32(b, 0x10))
	}
	ns := chunks(t, b)
	if len(ns) == 0 || ns[0].magic != "mhbd" || ns[0].end != len(b) {
		t.Fatalf("%+v", ns)
	}
}

func TestWriteEmptyLibrary(t *testing.T) {
	b := write(t, nil)
	chunks(t, b)
	if got := readDB(t, b); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
	if ids := masterPlaylistIDs(t, b); len(ids) != 0 {
		t.Fatalf("%v", ids)
	}
	for _, n := range chunks(t, b) {
		if n.magic == "mhyp" && le32(b, n.off+12) != 2 {
			t.Fatalf("empty mpl mhod count %d", le32(b, n.off+12))
		}
	}
}

func TestMacTime(t *testing.T) {
	if got := itunesdb.MacTime(time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)); got != 0 {
		t.Fatal(got)
	}
	if got := itunesdb.MacTime(time.Unix(0, 0).UTC()); got != 2082844800 {
		t.Fatal(got)
	}
}

func TestWriteMHBDLibgpodFields(t *testing.T) {
	b := write(t, sampleTracks())
	if le32(b, 0x0c) != 1 {
		t.Fatalf("unk 0x0c %d", le32(b, 0x0c))
	}
	if le32(b, 0x14) != 8 {
		t.Fatalf("children %d", le32(b, 0x14))
	}
	if le64(b, 0x18) != 99 {
		t.Fatalf("id %d", le64(b, 0x18))
	}
	if le16(b, 0x20) != 1 {
		t.Fatalf("platform %d", le16(b, 0x20))
	}
	if le16(b, 0x46) != 0x656e {
		t.Fatalf("lang %#x", le16(b, 0x46))
	}
	var types []uint32
	for _, n := range chunks(t, b) {
		if n.magic == "mhsd" {
			if le32(b, n.off+4) != 96 {
				t.Fatalf("mhsd header %d", le32(b, n.off+4))
			}
			types = append(types, le32(b, n.off+12))
		}
	}
	if !reflect.DeepEqual(types, []uint32{1, 3, 2, 4, 8, 6, 10, 5}) {
		t.Fatalf("mhsd order %v", types)
	}
}

func mhodString(b []byte, off int) string {
	l := int(le32(b, off+28))
	u := make([]uint16, l/2)
	for i := range u {
		u[i] = le16(b, off+40+2*i)
	}
	return string(utf16.Decode(u))
}

func TestWriteMHITLibgpodFields(t *testing.T) {
	when := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	tr := sampleTracks()
	tr[0].DateAdded = when
	tr[0].BitrateKbps = 900
	tr[0].Tags.Year = 2021
	tr[0].Tags.TrackTotal = 11
	tr[0].Tags.DiscTotal = 2
	tr[0].Tags.Compilation = true
	tr[0].Tags.Genre = "Metalcore"
	tr[0].Tags.Composer = "Daniel Pugsley"
	tr[0].Tags.SortTitle = "hourglass"
	tr[0].Tags.SortArtist = "Theory, Sleep"
	tr[0].Tags.SortAlbum = "afterglow"
	tr[0].Tags.SortAlbumArtist = "Theory, Sleep"
	b := write(t, tr)
	var mhits []node
	ns := chunks(t, b)
	for _, n := range ns {
		if n.magic == "mhit" {
			mhits = append(mhits, n)
		}
	}
	if len(mhits) != 2 {
		t.Fatalf("mhits %d", len(mhits))
	}
	o := mhits[0].off
	mt := itunesdb.MacTime(when)
	checks := []struct {
		name string
		got  uint64
		want uint64
	}{
		{"header", uint64(le32(b, o+4)), 0x248},
		{"id", uint64(le32(b, o+0x10)), 1},
		{"visible", uint64(le32(b, o+0x14)), 1},
		{"filetype", uint64(le32(b, o+0x18)), uint64(binary.BigEndian.Uint32([]byte("M4A ")))},
		{"type1", uint64(b[o+0x1c]), 0},
		{"type2", uint64(b[o+0x1d]), 0},
		{"compilation", uint64(b[o+0x1e]), 1},
		{"rating", uint64(b[o+0x1f]), 0},
		{"modified", uint64(le32(b, o+0x20)), uint64(mt)},
		{"size", uint64(le32(b, o+0x24)), 18000000},
		{"length", uint64(le32(b, o+0x28)), 164940},
		{"track", uint64(le32(b, o+0x2c)), 2},
		{"tracks", uint64(le32(b, o+0x30)), 11},
		{"year", uint64(le32(b, o+0x34)), 2021},
		{"bitrate", uint64(le32(b, o+0x38)), 900},
		{"samplerate", uint64(le32(b, o+0x3c)), 44100 << 16},
		{"playcount", uint64(le32(b, o+0x50)), 0},
		{"lastplayed", uint64(le32(b, o+0x58)), 0},
		{"cd", uint64(le32(b, o+0x5c)), 1},
		{"cds", uint64(le32(b, o+0x60)), 2},
		{"added", uint64(le32(b, o+0x68)), uint64(mt)},
		{"dbid", le64(b, o+0x70), 0xAABBCCDD11223344},
		{"artwork_count", uint64(le16(b, o+0x7c)), 1},
		{"artwork_size", uint64(le32(b, o+0x80)), 100000},
		{"samplerate2", uint64(le32(b, o+0x88)), uint64(math.Float32bits(44100))},
		{"has_artwork", uint64(b[o+0xa4]), 1},
		{"dbid2", le64(b, o+0xa8), 0xAABBCCDD11223344},
		{"mark_unplayed", uint64(b[o+0xb2]), 1},
		{"mediatype", uint64(le32(b, o+0xd0)), 1},
		{"album_id", uint64(le32(b, o+0x120)), 1},
		{"size2", uint64(le32(b, o+0x12c)), 18000000},
		{"magic808080", le64(b, o+0x134), 0x808080808080},
		{"mhii_link", uint64(le32(b, o+0x160)), 100},
		{"unk_0x168", uint64(le32(b, o+0x168)), 1},
		{"artist_id", uint64(le32(b, o+0x1e0)), 1},
		{"composer_id", uint64(le32(b, o+0x1f4)), 1},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %#x want %#x", c.name, c.got, c.want)
		}
	}
	o2 := mhits[1].off
	if b[o2+0xa4] != 2 || le16(b, o2+0x7c) != 0 || le32(b, o2+0x160) != 0 || le32(b, o2+0x68) != 0 || le32(b, o2+0x120) != 0 {
		t.Errorf("second track artwork/time fields wrong")
	}

	var types []uint32
	strs := map[uint32]string{}
	for _, n := range ns {
		if n.magic == "mhod" && n.off > mhits[0].off && n.off < mhits[0].end {
			typ := le32(b, n.off+12)
			types = append(types, typ)
			if le32(b, n.off+4) != 24 || le32(b, n.off+24) != 1 || le32(b, n.off+32) != 1 || int(le32(b, n.off+8)) != 40+int(le32(b, n.off+28)) {
				t.Errorf("mhod %d header wrong", typ)
			}
			strs[typ] = mhodString(b, n.off)
		}
	}
	if !reflect.DeepEqual(types, []uint32{1, 4, 3, 6, 2, 5, 12, 22, 23, 27, 28, 29}) {
		t.Fatalf("mhod order %v", types)
	}
	if int(le32(b, mhits[0].off+12)) != len(types) {
		t.Fatalf("mhod count %d", le32(b, mhits[0].off+12))
	}
	if strs[6] != "Apple Lossless audio file" || strs[5] != "Metalcore" || strs[12] != "Daniel Pugsley" || strs[23] != "Theory, Sleep" {
		t.Fatalf("%v", strs)
	}
}

func mplChildren(t *testing.T, b []byte, mhsdType uint32) (node, []node) {
	t.Helper()
	ns := chunks(t, b)
	for i, n := range ns {
		if n.magic != "mhsd" || le32(b, n.off+12) != mhsdType {
			continue
		}
		for j := i + 1; j < len(ns) && ns[j].off < n.end; j++ {
			if ns[j].magic == "mhyp" && b[ns[j].off+0x14] == 1 {
				yp := ns[j]
				var kids []node
				for k := j + 1; k < len(ns) && ns[k].off < yp.end; k++ {
					if ns[k].depth == yp.depth+1 {
						kids = append(kids, ns[k])
					}
				}
				return yp, kids
			}
		}
	}
	t.Fatal("no master playlist")
	return node{}, nil
}

func TestWriteMasterPlaylistLayout(t *testing.T) {
	b := write(t, sampleTracks())
	for _, st := range []uint32{2, 3} {
		yp, kids := mplChildren(t, b, st)
		o := yp.off
		if le32(b, o+4) != 108 || le32(b, o+12) != 12 || le32(b, o+16) != 2 {
			t.Fatalf("mhyp header %d mhods %d mhips %d", le32(b, o+4), le32(b, o+12), le32(b, o+16))
		}
		if le16(b, o+0x28) != 1 || le32(b, o+0x2c) != 1 {
			t.Fatalf("mhyp string count %d sortorder %d", le16(b, o+0x28), le32(b, o+0x2c))
		}
		var mhodTypes []uint32
		var mhips []node
		for _, k := range kids {
			switch k.magic {
			case "mhod":
				mhodTypes = append(mhodTypes, le32(b, k.off+12))
			case "mhip":
				mhips = append(mhips, k)
			}
		}
		if !reflect.DeepEqual(mhodTypes, []uint32{1, 100, 52, 53, 52, 53, 52, 53, 52, 53, 52, 53}) {
			t.Fatalf("mpl mhods %v", mhodTypes)
		}
		if mhodString(b, kids[0].off) != "iPod" {
			t.Fatalf("mpl title %q", mhodString(b, kids[0].off))
		}
		if le32(b, kids[1].off+8) != 0x288 {
			t.Fatalf("long mhod 100 length %#x", le32(b, kids[1].off+8))
		}
		for i, ip := range mhips {
			if le32(b, ip.off+4) != 76 || le32(b, ip.off+12) != 1 || le32(b, ip.off+24) != uint32(i+1) {
				t.Fatalf("mhip %d wrong", i)
			}
			od := ip.off + 76
			if string(b[od:od+4]) != "mhod" || le32(b, od+8) != 44 || le32(b, od+12) != 100 || le32(b, od+24) != uint32(i) {
				t.Fatalf("mhip %d mhod wrong", i)
			}
		}
	}
}

func sortIndex(t *testing.T, b []byte) (map[uint32][]uint32, map[uint32][][3]uint32) {
	t.Helper()
	_, kids := mplChildren(t, b, 2)
	idx := map[uint32][]uint32{}
	jump := map[uint32][][3]uint32{}
	for _, k := range kids {
		if k.magic != "mhod" {
			continue
		}
		o := k.off
		switch le32(b, o+12) {
		case 52:
			st, n := le32(b, o+24), int(le32(b, o+28))
			if int(le32(b, o+8)) != 72+4*n {
				t.Fatalf("mhod52 length")
			}
			var l []uint32
			for i := 0; i < n; i++ {
				l = append(l, le32(b, o+72+4*i))
			}
			idx[st] = l
		case 53:
			st, n := le32(b, o+24), int(le32(b, o+28))
			if int(le32(b, o+8)) != 40+12*n {
				t.Fatalf("mhod53 length")
			}
			var l [][3]uint32
			for i := 0; i < n; i++ {
				e := o + 40 + 12*i
				l = append(l, [3]uint32{uint32(le16(b, e)), le32(b, e+4), le32(b, e+8)})
			}
			jump[st] = l
		}
	}
	return idx, jump
}

func TestWriteMasterPlaylistSortIndices(t *testing.T) {
	tr := []itunesdb.Track{
		{ID: 1, DBID: 11, Path: ":a", Tags: library.Tags{Title: "banana", Artist: "The Zeds", Album: "Zulu", Genre: "rock", Composer: "x", TrackNumber: 2}},
		{ID: 2, DBID: 12, Path: ":b", Tags: library.Tags{Title: "Apple", Artist: "beta", Album: "zulu", Genre: "Ambient", TrackNumber: 1, SortTitle: "Cherry"}},
		{ID: 3, DBID: 13, Path: ":c", Tags: library.Tags{Title: "cherry", Artist: "Alpha", Album: "", Genre: "Rock", Composer: "Y"}},
		{ID: 4, DBID: 14, Path: ":d", Tags: library.Tags{Title: "1999", Artist: "alpha", Album: "Zulu", SortAlbum: "Echo", TrackNumber: 3}},
	}
	b := write(t, tr)
	idx, jump := sortIndex(t, b)
	want := map[uint32][]uint32{
		3:    {3, 0, 2, 1},
		5:    {2, 3, 1, 0},
		4:    {2, 3, 1, 0},
		7:    {3, 1, 2, 0},
		0x12: {3, 1, 0, 2},
	}
	if !reflect.DeepEqual(idx, want) {
		t.Fatalf("indices %v", idx)
	}
	wantJump := map[uint32][][3]uint32{
		3:    {{'0', 0, 1}, {'B', 1, 1}, {'C', 2, 2}},
		5:    {{'A', 0, 2}, {'B', 2, 1}, {'Z', 3, 1}},
		4:    {{'0', 0, 1}, {'E', 1, 1}, {'Z', 2, 2}},
		7:    {{'0', 0, 1}, {'A', 1, 1}, {'R', 2, 2}},
		0x12: {{'0', 0, 2}, {'X', 2, 1}, {'Y', 3, 1}},
	}
	if !reflect.DeepEqual(jump, wantJump) {
		t.Fatalf("jump %v", jump)
	}
}

func TestWriteAlbumAndArtistLists(t *testing.T) {
	tr := []itunesdb.Track{
		{ID: 1, DBID: 1, Path: ":a", Tags: library.Tags{Title: "a", Artist: "X", AlbumArtist: "AA", Album: "One"}},
		{ID: 2, DBID: 2, Path: ":b", Tags: library.Tags{Title: "b", Artist: "Y", AlbumArtist: "AA", Album: "One"}},
		{ID: 3, DBID: 3, Path: ":c", Tags: library.Tags{Title: "c", Artist: "X", Album: "Two"}},
		{ID: 4, DBID: 4, Path: ":d", Tags: library.Tags{Title: "d"}},
	}
	b := write(t, tr)
	ns := chunks(t, b)
	var albumIDs, artistIDs []uint32
	var mhias, mhiis []node
	for _, n := range ns {
		switch n.magic {
		case "mhit":
			albumIDs = append(albumIDs, le32(b, n.off+0x120))
			artistIDs = append(artistIDs, le32(b, n.off+0x1e0))
		case "mhia":
			mhias = append(mhias, n)
		case "mhii":
			mhiis = append(mhiis, n)
		case "mhla", "mhli":
			if le32(b, n.off+4) != 92 {
				t.Fatalf("%s header", n.magic)
			}
		}
	}
	if !reflect.DeepEqual(albumIDs, []uint32{1, 1, 2, 0}) || !reflect.DeepEqual(artistIDs, []uint32{1, 2, 1, 0}) {
		t.Fatalf("album %v artist %v", albumIDs, artistIDs)
	}
	if len(mhias) != 2 || len(mhiis) != 2 {
		t.Fatalf("mhia %d mhii %d", len(mhias), len(mhiis))
	}
	if le32(b, mhias[0].off+4) != 88 || le32(b, mhias[0].off+16) != 1 || le32(b, mhias[0].off+28) != 2 || le32(b, mhias[0].off+12) != 2 {
		t.Fatalf("mhia header")
	}
	if le32(b, mhiis[1].off+4) != 80 || le32(b, mhiis[1].off+16) != 2 || le32(b, mhiis[1].off+12) != 1 {
		t.Fatalf("mhii header")
	}
	var s []string
	for _, n := range ns {
		if n.magic == "mhod" && n.off > mhias[0].off && n.off < mhias[1].end {
			s = append(s, mhodString(b, n.off))
		}
		if n.magic == "mhod" && n.off > mhiis[1].off && n.off < mhiis[1].end {
			if le32(b, n.off+12) != 300 {
				t.Fatalf("mhii mhod type %d", le32(b, n.off+12))
			}
			s = append(s, mhodString(b, n.off))
		}
	}
	if !reflect.DeepEqual(s, []string{"One", "AA", "Two", "X", "Y"}) {
		t.Fatalf("%q", s)
	}
}

func TestWriteRejectsOversizeFile(t *testing.T) {
	tr := sampleTracks()
	tr[0].Size = 1 << 32
	var b bytes.Buffer
	if err := itunesdb.Write(&b, tr, 1); err == nil {
		t.Fatal("expected error")
	}
}
