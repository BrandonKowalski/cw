package artwork

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
)

type Format struct {
	ID   uint32
	W, H int
}

var Formats = []Format{{1028, 100, 100}, {1029, 200, 200}}

func ID(img []byte) string {
	sum := sha256.Sum256(img)
	return hex.EncodeToString(sum[:])
}

func Render(img []byte, f Format) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(img))
	if err != nil {
		return nil, fmt.Errorf("decode cover art: %w", err)
	}
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	if side <= 0 || f.W <= 0 || f.H <= 0 {
		return nil, fmt.Errorf("render cover art: empty image or format")
	}
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	crop := image.Rect(x0, y0, x0+side, y0+side)
	dst := image.NewRGBA(image.Rect(0, 0, f.W, f.H))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Src, nil)
	out := make([]byte, f.W*f.H*2)
	for y := 0; y < f.H; y++ {
		row := dst.Pix[y*dst.Stride:]
		for x := 0; x < f.W; x++ {
			p := row[x*4:]
			v := uint16(p[0]>>3)<<11 | uint16(p[1]>>2)<<5 | uint16(p[2]>>3)
			binary.LittleEndian.PutUint16(out[(y*f.W+x)*2:], v)
		}
	}
	return out, nil
}
