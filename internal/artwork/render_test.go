package artwork_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/BrandonKowalski/cw/internal/artwork"
)

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestRenderRGB565(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{255, 0, 0, 255}}, image.Point{}, draw.Src)
	out, err := artwork.Render(encodePNG(t, img), artwork.Format{ID: 1028, W: 100, H: 100})
	if err != nil || len(out) != 20000 {
		t.Fatal(len(out), err)
	}
	if binary.LittleEndian.Uint16(out[0:2]) != 0xF800 {
		t.Fatalf("%04x", binary.LittleEndian.Uint16(out[0:2]))
	}
}

func TestRenderCenterCropAndPacking(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	draw.Draw(img, image.Rect(0, 0, 100, 200), &image.Uniform{color.RGBA{0, 0, 255, 255}}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(100, 0, 300, 100), &image.Uniform{color.RGBA{0, 255, 0, 255}}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(100, 100, 300, 200), &image.Uniform{color.RGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(300, 0, 400, 200), &image.Uniform{color.RGBA{0, 0, 255, 255}}, image.Point{}, draw.Src)
	f := artwork.Format{ID: 1029, W: 200, H: 200}
	out, err := artwork.Render(encodePNG(t, img), f)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != f.W*f.H*2 {
		t.Fatal(len(out))
	}
	px := func(x, y int) uint16 { return binary.LittleEndian.Uint16(out[(y*f.W+x)*2:]) }
	if px(0, 0) != 0x07E0 || px(199, 50) != 0x07E0 {
		t.Fatalf("top %04x %04x", px(0, 0), px(199, 50))
	}
	if px(0, 199) != 0xFFFF || px(150, 150) != 0xFFFF {
		t.Fatalf("bottom %04x %04x", px(0, 199), px(150, 150))
	}
}

func TestRenderJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 50, 80))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{0, 0, 255, 255}}, image.Point{}, draw.Src)
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	out, err := artwork.Render(b.Bytes(), artwork.Formats[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 20000 {
		t.Fatal(len(out))
	}
	p := binary.LittleEndian.Uint16(out[10100:])
	if p>>11 > 1 || (p>>5)&0x3F > 2 || p&0x1F < 0x1E {
		t.Fatalf("%04x", p)
	}
}

func TestRenderRejectsGarbage(t *testing.T) {
	if _, err := artwork.Render([]byte("not an image at all"), artwork.Formats[0]); err == nil {
		t.Fatal("garbage accepted")
	}
	if _, err := artwork.Render(nil, artwork.Formats[0]); err == nil {
		t.Fatal("empty accepted")
	}
}

func TestFormats(t *testing.T) {
	want := []artwork.Format{{1028, 100, 100}, {1029, 200, 200}}
	if len(artwork.Formats) != 2 || artwork.Formats[0] != want[0] || artwork.Formats[1] != want[1] {
		t.Fatal(artwork.Formats)
	}
}

func TestID(t *testing.T) {
	if got := artwork.ID([]byte("abc")); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatal(got)
	}
}
