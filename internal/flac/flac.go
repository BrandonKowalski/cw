package flac

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

type StreamInfo struct {
	SampleRate    int
	BitsPerSample int
	Channels      int
	TotalSamples  uint64
	MD5           [16]byte
}

type Picture struct {
	Type uint32
	MIME string
	Data []byte
}

type Metadata struct {
	Stream      StreamInfo
	Tags        map[string][]string
	Pictures    []Picture
	AudioOffset int64
}

func ReadFile(path string) (*Metadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var pos int64

	head := make([]byte, 4)
	if _, err := io.ReadFull(r, head); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	pos = 4
	if string(head[:3]) == "ID3" {
		rest := make([]byte, 6)
		if _, err := io.ReadFull(r, rest); err != nil {
			return nil, fmt.Errorf("read id3: %w", err)
		}
		size := int64(rest[2]&0x7f)<<21 | int64(rest[3]&0x7f)<<14 | int64(rest[4]&0x7f)<<7 | int64(rest[5]&0x7f)
		if rest[1]&0x10 != 0 {
			size += 10
		}
		if _, err := io.CopyN(io.Discard, r, size); err != nil {
			return nil, fmt.Errorf("skip id3: %w", err)
		}
		pos = 10 + size
		if _, err := io.ReadFull(r, head); err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}
		pos += 4
	}
	if string(head) != "fLaC" {
		return nil, errors.New("not a FLAC file")
	}

	m := &Metadata{Tags: map[string][]string{}}
	sawStream := false
	for {
		var bh [4]byte
		if _, err := io.ReadFull(r, bh[:]); err != nil {
			return nil, fmt.Errorf("read block header: %w", err)
		}
		pos += 4
		last := bh[0]&0x80 != 0
		typ := bh[0] & 0x7f
		length := int64(bh[1])<<16 | int64(bh[2])<<8 | int64(bh[3])
		switch typ {
		case 0, 4, 6:
			buf := make([]byte, length)
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, fmt.Errorf("read block: %w", err)
			}
			var perr error
			switch typ {
			case 0:
				perr = parseStreamInfo(buf, &m.Stream)
				sawStream = perr == nil
			case 4:
				perr = parseVorbis(buf, m.Tags)
			case 6:
				var p Picture
				p, perr = parsePicture(buf)
				if perr == nil {
					m.Pictures = append(m.Pictures, p)
				}
			}
			if perr != nil {
				return nil, perr
			}
		default:
			if _, err := io.CopyN(io.Discard, r, length); err != nil {
				return nil, fmt.Errorf("skip block: %w", err)
			}
		}
		pos += length
		if last {
			break
		}
	}
	if !sawStream {
		return nil, errors.New("missing STREAMINFO")
	}
	m.AudioOffset = pos
	return m, nil
}

func parseStreamInfo(b []byte, s *StreamInfo) error {
	if len(b) < 34 {
		return errors.New("short STREAMINFO")
	}
	v := binary.BigEndian.Uint64(b[10:18])
	s.SampleRate = int(v >> 44)
	s.Channels = int(v>>41&0x7) + 1
	s.BitsPerSample = int(v>>36&0x1f) + 1
	s.TotalSamples = v & 0xfffffffff
	copy(s.MD5[:], b[18:34])
	return nil
}

func parseVorbis(b []byte, tags map[string][]string) error {
	errShort := errors.New("short VORBIS_COMMENT")
	if len(b) < 4 {
		return errShort
	}
	vl := int(binary.LittleEndian.Uint32(b))
	off := 4 + vl
	if vl < 0 || off+4 > len(b) {
		return errShort
	}
	n := int(binary.LittleEndian.Uint32(b[off:]))
	off += 4
	for i := 0; i < n; i++ {
		if off+4 > len(b) {
			return errShort
		}
		l := int(binary.LittleEndian.Uint32(b[off:]))
		off += 4
		if l < 0 || off+l > len(b) {
			return errShort
		}
		k, v, ok := strings.Cut(string(b[off:off+l]), "=")
		off += l
		if !ok {
			continue
		}
		k = strings.ToUpper(k)
		tags[k] = append(tags[k], v)
	}
	return nil
}

func parsePicture(b []byte) (Picture, error) {
	errShort := errors.New("short PICTURE")
	var p Picture
	if len(b) < 8 {
		return p, errShort
	}
	p.Type = binary.BigEndian.Uint32(b)
	ml := int(binary.BigEndian.Uint32(b[4:]))
	off := 8
	if ml < 0 || off+ml+4 > len(b) {
		return p, errShort
	}
	p.MIME = string(b[off : off+ml])
	off += ml
	dl := int(binary.BigEndian.Uint32(b[off:]))
	off += 4
	if dl < 0 || off+dl+20 > len(b) {
		return p, errShort
	}
	off += dl + 16
	n := int(binary.BigEndian.Uint32(b[off:]))
	off += 4
	if n < 0 || off+n > len(b) {
		return p, errShort
	}
	p.Data = b[off : off+n]
	return p, nil
}
