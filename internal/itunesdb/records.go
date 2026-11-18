package itunesdb

import (
	"encoding/binary"
	"math"
	"unicode/utf16"
)

const (
	mhodTitle           = 1
	mhodPath            = 2
	mhodAlbum           = 3
	mhodArtist          = 4
	mhodGenre           = 5
	mhodFiletype        = 6
	mhodComposer        = 12
	mhodAlbumArtist     = 22
	mhodSortArtist      = 23
	mhodSortTitle       = 27
	mhodSortAlbum       = 28
	mhodSortAlbumArtist = 29
	mhodTypeLibIndex    = 52
	mhodTypeJumpTable   = 53
	mhodPlaylist        = 100
	mhodAlbumAlbum      = 200
	mhodAlbumArtistName = 201
	mhodAlbumSortArtist = 202
	mhodArtistName      = 300
)

type buf struct {
	b []byte
}

func (w *buf) magic(s string) { w.b = append(w.b, s...) }
func (w *buf) u8(v uint8)     { w.b = append(w.b, v) }
func (w *buf) u16(v uint16)   { w.b = binary.LittleEndian.AppendUint16(w.b, v) }
func (w *buf) u32(v uint32)   { w.b = binary.LittleEndian.AppendUint32(w.b, v) }
func (w *buf) u64(v uint64)   { w.b = binary.LittleEndian.AppendUint64(w.b, v) }
func (w *buf) zeros(n int)    { w.b = append(w.b, make([]byte, n)...) }
func (w *buf) raw(p []byte)   { w.b = append(w.b, p...) }

func (w *buf) put32At(off int, v uint32) {
	binary.LittleEndian.PutUint32(w.b[off:], v)
}

func concat(parts [][]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

type mhbdFields struct {
	children uint32
	id       uint64
	pid      uint64
}

func mhbd(f mhbdFields, body []byte) []byte {
	var w buf
	w.magic("mhbd")
	w.u32(244)
	w.u32(0)
	w.u32(1)
	w.u32(0x30)
	w.u32(f.children)
	w.u64(f.id)
	w.u16(1)
	w.u16(0)
	w.u64(0)
	w.u32(0)
	w.u16(0)
	w.zeros(20)
	w.u16(0x656e)
	w.u64(f.pid)
	w.u32(0)
	w.u32(0)
	w.zeros(20)
	w.u32(0)
	w.u16(0)
	w.u16(0)
	w.zeros(44)
	w.u16(0)
	w.u16(0)
	w.u16(0)
	w.u16(0)
	w.u16(0)
	w.u8(0)
	w.u8(0)
	w.zeros(56)
	w.zeros(16)
	w.raw(body)
	w.put32At(8, uint32(len(w.b)))
	return w.b
}

func mhsd(typ uint32, body []byte) []byte {
	var w buf
	w.magic("mhsd")
	w.u32(96)
	w.u32(uint32(96 + len(body)))
	w.u32(typ)
	w.zeros(80)
	w.raw(body)
	return w.b
}

func listHeader(magic string, count int) []byte {
	var w buf
	w.magic(magic)
	w.u32(92)
	w.u32(uint32(count))
	w.zeros(80)
	return w.b
}

type mhitFields struct {
	id           uint32
	filetype     uint32
	compilation  uint8
	modified     uint32
	size         uint32
	lengthMS     uint32
	trackNr      uint32
	tracks       uint32
	year         uint32
	bitrate      uint32
	sampleRate   uint32
	cdNr         uint32
	cds          uint32
	added        uint32
	dbid         uint64
	artworkCount uint16
	artworkSize  uint32
	hasArtwork   uint8
	albumID      uint32
	mhiiLink     uint32
	artistID     uint32
	composerID   uint32
}

func mhit(f mhitFields, mhods [][]byte) []byte {
	var w buf
	w.magic("mhit")
	w.u32(0x248)
	w.u32(0)
	w.u32(uint32(len(mhods)))
	w.u32(f.id)
	w.u32(1)
	w.u32(f.filetype)
	w.u8(0)
	w.u8(0)
	w.u8(f.compilation)
	w.u8(0)
	w.u32(f.modified)
	w.u32(f.size)
	w.u32(f.lengthMS)
	w.u32(f.trackNr)
	w.u32(f.tracks)
	w.u32(f.year)
	w.u32(f.bitrate)
	w.u32(f.sampleRate << 16)
	w.u32(0)
	w.u32(0)
	w.u32(0)
	w.u32(0)
	w.u32(0)
	w.u32(0)
	w.u32(0)
	w.u32(f.cdNr)
	w.u32(f.cds)
	w.u32(0)
	w.u32(f.added)
	w.u32(0)
	w.u64(f.dbid)
	w.u8(0)
	w.u8(0)
	w.u16(0)
	w.u16(f.artworkCount)
	w.u16(0)
	w.u32(f.artworkSize)
	w.u32(0)
	w.u32(math.Float32bits(float32(f.sampleRate)))
	w.u32(0)
	w.u16(0)
	w.u16(0)
	w.u32(0)
	w.u32(0)
	w.u32(0)
	w.u32(0)
	w.u8(f.hasArtwork)
	w.u8(0)
	w.u8(0)
	w.u8(0)
	w.u64(f.dbid)
	w.u8(0)
	w.u8(0)
	w.u8(1)
	w.u8(0)
	w.u32(0)
	w.u32(0)
	w.u64(0)
	w.u32(0)
	w.u32(0)
	w.u32(0)
	w.u32(1)
	w.u32(0)
	w.u32(0)
	w.u32(0)
	w.zeros(16)
	w.zeros(16)
	w.u16(0)
	w.u16(0)
	w.zeros(28)
	w.u32(f.albumID)
	w.u64(0)
	w.u32(f.size)
	w.u32(0)
	w.u64(0x808080808080)
	w.u32(0)
	w.zeros(8)
	w.u32(0)
	w.zeros(20)
	w.u32(f.mhiiLink)
	w.u32(0)
	w.u32(1)
	w.u32(0)
	w.zeros(112)
	w.u32(f.artistID)
	w.zeros(16)
	w.u32(f.composerID)
	w.zeros(80)
	w.raw(concat(mhods))
	w.put32At(8, uint32(len(w.b)))
	return w.b
}

func mhodString(typ uint32, s string) []byte {
	u := utf16.Encode([]rune(s))
	var w buf
	w.magic("mhod")
	w.u32(24)
	w.u32(uint32(2*len(u) + 40))
	w.u32(typ)
	w.zeros(8)
	w.u32(1)
	w.u32(uint32(2 * len(u)))
	w.u32(1)
	w.u32(0)
	for _, c := range u {
		w.u16(c)
	}
	return w.b
}

func mhodPlaylistPos(pos uint32) []byte {
	var w buf
	w.magic("mhod")
	w.u32(24)
	w.u32(44)
	w.u32(mhodPlaylist)
	w.zeros(8)
	w.u32(pos)
	w.zeros(16)
	return w.b
}

func mhodPlaylistPrefs() []byte {
	var w buf
	w.magic("mhod")
	w.u32(0x18)
	w.u32(0x288)
	w.u32(mhodPlaylist)
	w.zeros(24)
	w.u32(0x010084)
	w.u32(0x05)
	w.u32(0x09)
	w.u32(0x03)
	w.u32(0x120001)
	w.zeros(12)
	w.u32(0xc80002)
	w.zeros(12)
	w.u32(0x3c000d)
	w.zeros(12)
	w.u32(0x7d0004)
	w.zeros(12)
	w.u32(0x7d0003)
	w.zeros(12)
	w.u32(0x640008)
	w.zeros(12)
	w.u32(0x640017)
	w.u32(0x01)
	w.zeros(8)
	w.u32(0x500014)
	w.u32(0x01)
	w.zeros(8)
	w.u32(0x7d0015)
	w.u32(0x01)
	w.zeros(456)
	return w.b
}

func mhodLibIndex(sortType uint32, indices []uint32) []byte {
	var w buf
	w.magic("mhod")
	w.u32(24)
	w.u32(uint32(4*len(indices) + 72))
	w.u32(mhodTypeLibIndex)
	w.zeros(8)
	w.u32(sortType)
	w.u32(uint32(len(indices)))
	w.zeros(40)
	for _, i := range indices {
		w.u32(i)
	}
	return w.b
}

type jumpEntry struct {
	letter uint16
	start  uint32
	count  uint32
}

func mhodJumpTable(sortType uint32, entries []jumpEntry) []byte {
	var w buf
	w.magic("mhod")
	w.u32(24)
	w.u32(uint32(12*len(entries) + 40))
	w.u32(mhodTypeJumpTable)
	w.zeros(8)
	w.u32(sortType)
	w.u32(uint32(len(entries)))
	w.zeros(8)
	for _, e := range entries {
		w.u16(e.letter)
		w.u16(0)
		w.u32(e.start)
		w.u32(e.count)
	}
	return w.b
}

func mhip(trackID, pos uint32) []byte {
	var w buf
	w.magic("mhip")
	w.u32(76)
	w.u32(0)
	w.u32(1)
	w.u32(0)
	w.u32(0)
	w.u32(trackID)
	w.u32(0)
	w.u32(0)
	w.zeros(40)
	w.raw(mhodPlaylistPos(pos))
	w.put32At(8, uint32(len(w.b)))
	return w.b
}

type mhypFields struct {
	master    bool
	timestamp uint32
	id        uint64
	mhips     int
}

func mhyp(f mhypFields, mhods [][]byte, mhips [][]byte) []byte {
	var w buf
	w.magic("mhyp")
	w.u32(108)
	w.u32(0)
	w.u32(uint32(len(mhods)))
	w.u32(uint32(f.mhips))
	if f.master {
		w.u8(1)
	} else {
		w.u8(0)
	}
	w.u8(0)
	w.u8(0)
	w.u8(0)
	w.u32(f.timestamp)
	w.u64(f.id)
	w.u32(0)
	w.u16(1)
	w.u16(0)
	w.u32(1)
	w.zeros(60)
	w.raw(concat(mhods))
	w.raw(concat(mhips))
	w.put32At(8, uint32(len(w.b)))
	return w.b
}

func mhia(id uint32, sqlID uint64, mhods [][]byte) []byte {
	var w buf
	w.magic("mhia")
	w.u32(88)
	w.u32(0)
	w.u32(uint32(len(mhods)))
	w.u32(id)
	w.u64(sqlID)
	w.u32(2)
	w.zeros(56)
	w.raw(concat(mhods))
	w.put32At(8, uint32(len(w.b)))
	return w.b
}

func mhii(id uint32, sqlID uint64, mhods [][]byte) []byte {
	var w buf
	w.magic("mhii")
	w.u32(80)
	w.u32(0)
	w.u32(uint32(len(mhods)))
	w.u32(id)
	w.u64(sqlID)
	w.u32(2)
	w.zeros(48)
	w.raw(concat(mhods))
	w.put32At(8, uint32(len(w.b)))
	return w.b
}
