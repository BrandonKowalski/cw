package itunesdbtest

import (
	"encoding/binary"
	"fmt"
	"unicode/utf16"
)

type Track struct {
	ID          uint32
	DBID        uint64
	Path        string
	Title       string
	Album       string
	Artist      string
	AlbumArtist string
	TrackNumber int
	DiscNumber  int
	DurationMS  int
	SampleRate  int
	Size        int64
	MHIILink    uint32
}

type chunk struct {
	magic  string
	off    int
	header int
	total  int
}

func readChunk(b []byte, off, limit int) (chunk, error) {
	if off < 0 || off+12 > limit || limit > len(b) {
		return chunk{}, fmt.Errorf("chunk at %d out of bounds", off)
	}
	c := chunk{
		magic:  string(b[off : off+4]),
		off:    off,
		header: int(binary.LittleEndian.Uint32(b[off+4:])),
		total:  int(binary.LittleEndian.Uint32(b[off+8:])),
	}
	if c.header < 12 || c.header > limit-off {
		return chunk{}, fmt.Errorf("%s at %d: bad header length %d", c.magic, off, c.header)
	}
	return c, nil
}

func (c chunk) end(limit int) (int, error) {
	if c.total < c.header || c.off+c.total > limit {
		return 0, fmt.Errorf("%s at %d: bad total length %d", c.magic, c.off, c.total)
	}
	return c.off + c.total, nil
}

func u32(b []byte, off int) uint32 {
	return binary.LittleEndian.Uint32(b[off:])
}

func mhsds(b []byte) (map[uint32]chunk, error) {
	db, err := readChunk(b, 0, len(b))
	if err != nil {
		return nil, err
	}
	if db.magic != "mhbd" {
		return nil, fmt.Errorf("not an iTunesDB: %q", db.magic)
	}
	end, err := db.end(len(b))
	if err != nil {
		return nil, err
	}
	out := map[uint32]chunk{}
	for off := db.header; off < end; {
		c, err := readChunk(b, off, end)
		if err != nil {
			return nil, err
		}
		if c.magic != "mhsd" {
			return nil, fmt.Errorf("expected mhsd at %d, got %q", off, c.magic)
		}
		next, err := c.end(end)
		if err != nil {
			return nil, err
		}
		t := u32(b, off+12)
		if _, ok := out[t]; !ok {
			out[t] = c
		}
		off = next
	}
	return out, nil
}

func Read(b []byte) ([]Track, error) {
	sds, err := mhsds(b)
	if err != nil {
		return nil, err
	}
	sd, ok := sds[1]
	if !ok {
		return nil, fmt.Errorf("no track mhsd")
	}
	sdEnd := sd.off + sd.total
	lt, err := readChunk(b, sd.off+sd.header, sdEnd)
	if err != nil {
		return nil, err
	}
	if lt.magic != "mhlt" {
		return nil, fmt.Errorf("expected mhlt, got %q", lt.magic)
	}
	n := int(lt.total)
	var tracks []Track
	off := lt.off + lt.header
	for i := 0; i < n; i++ {
		it, err := readChunk(b, off, sdEnd)
		if err != nil {
			return nil, err
		}
		if it.magic != "mhit" || it.header < 0x164 {
			return nil, fmt.Errorf("bad mhit at %d", off)
		}
		itEnd, err := it.end(sdEnd)
		if err != nil {
			return nil, err
		}
		t := Track{
			ID:          u32(b, off+0x10),
			Size:        int64(u32(b, off+0x24)),
			DurationMS:  int(u32(b, off+0x28)),
			TrackNumber: int(u32(b, off+0x2c)),
			SampleRate:  int(u32(b, off+0x3c) >> 16),
			DiscNumber:  int(u32(b, off+0x5c)),
			DBID:        binary.LittleEndian.Uint64(b[off+0x70:]),
			MHIILink:    u32(b, off+0x160),
		}
		nmhod := int(u32(b, off+12))
		mo := off + it.header
		for j := 0; j < nmhod; j++ {
			od, err := readChunk(b, mo, itEnd)
			if err != nil {
				return nil, err
			}
			odEnd, err := od.end(itEnd)
			if err != nil {
				return nil, err
			}
			if od.magic != "mhod" {
				return nil, fmt.Errorf("expected mhod at %d, got %q", mo, od.magic)
			}
			typ := u32(b, mo+12)
			if typ < 50 && od.total >= 40 {
				l := int(u32(b, mo+28))
				if mo+40+l > odEnd || l%2 != 0 {
					return nil, fmt.Errorf("bad mhod string at %d", mo)
				}
				s := utf16le(b[mo+40 : mo+40+l])
				switch typ {
				case 1:
					t.Title = s
				case 2:
					t.Path = s
				case 3:
					t.Album = s
				case 4:
					t.Artist = s
				case 22:
					t.AlbumArtist = s
				}
			}
			mo = odEnd
		}
		tracks = append(tracks, t)
		off = itEnd
	}
	return tracks, nil
}

func utf16le(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

func MasterPlaylistIDs(b []byte) ([]uint32, error) {
	sds, err := mhsds(b)
	if err != nil {
		return nil, err
	}
	sd, ok := sds[2]
	if !ok {
		return nil, fmt.Errorf("no playlist mhsd")
	}
	sdEnd := sd.off + sd.total
	lp, err := readChunk(b, sd.off+sd.header, sdEnd)
	if err != nil {
		return nil, err
	}
	if lp.magic != "mhlp" {
		return nil, fmt.Errorf("expected mhlp, got %q", lp.magic)
	}
	off := lp.off + lp.header
	for i := 0; i < lp.total; i++ {
		yp, err := readChunk(b, off, sdEnd)
		if err != nil {
			return nil, err
		}
		ypEnd, err := yp.end(sdEnd)
		if err != nil {
			return nil, err
		}
		if yp.magic != "mhyp" || yp.header < 0x18 {
			return nil, fmt.Errorf("bad mhyp at %d", off)
		}
		if b[off+0x14] != 1 {
			off = ypEnd
			continue
		}
		ids := []uint32{}
		for co := off + yp.header; co < ypEnd; {
			c, err := readChunk(b, co, ypEnd)
			if err != nil {
				return nil, err
			}
			next, err := c.end(ypEnd)
			if err != nil {
				return nil, err
			}
			if c.magic == "mhip" {
				if c.header < 28 {
					return nil, fmt.Errorf("bad mhip at %d", co)
				}
				ids = append(ids, u32(b, co+24))
			}
			co = next
		}
		return ids, nil
	}
	return nil, fmt.Errorf("no master playlist")
}
