package image

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"regexp"
	"strings"
	"testing"
)

func TestContainRect(t *testing.T) {
	cases := []struct {
		src  image.Point
		w, h int
		want image.Rectangle
	}{
		{image.Pt(96, 96), 34, 37, image.Rect(0, 1, 34, 35)},
		{image.Pt(96, 96), 40, 30, image.Rect(5, 0, 35, 30)},
		{image.Pt(200, 100), 100, 100, image.Rect(0, 25, 100, 75)},
		{image.Pt(100, 100), 100, 100, image.Rect(0, 0, 100, 100)},
		{image.Pt(0, 0), 10, 20, image.Rect(0, 0, 10, 20)},
	}
	for _, c := range cases {
		if got := containRect(c.src, c.w, c.h); got != c.want {
			t.Errorf("containRect(%v, %d, %d) = %v, want %v", c.src, c.w, c.h, got, c.want)
		}
	}
}

// A cell is rarely exactly twice as tall as wide. A square image in two
// cells of one row must stay square there, not be stretched over cells
// 17x37 px, which drew every emoji and avatar taller than wide.
func TestKitty_UploadKeepsTheImagesProportions(t *testing.T) {
	t.Setenv("TMUX", "")
	oldW, oldH := measuredCellPixels()
	defer SetCellPixels(oldW, oldH)
	SetCellPixels(17, 37)

	red := color.NRGBA{R: 200, A: 255}
	src := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	for i := range src.Pix {
		src.Pix[i] = []uint8{red.R, red.G, red.B, red.A}[i%4]
	}
	out := NewKittyRenderer(NewRegistry()).Render(src, image.Pt(2, 1))
	if out.OnFlush == nil {
		t.Fatal("a new image has no upload")
	}
	var upload bytes.Buffer
	if err := out.OnFlush(&upload); err != nil {
		t.Fatal(err)
	}
	var payload strings.Builder
	for _, m := range regexp.MustCompile(`\x1b_G[^;\x1b]*;([^\x1b]*)\x1b\\`).FindAllStringSubmatch(upload.String(), -1) {
		payload.WriteString(m[1])
	}
	raw, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil {
		t.Fatalf("payload is not base64: %v", err)
	}
	sent, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("payload is not a PNG: %v", err)
	}
	if got := sent.Bounds().Size(); got != image.Pt(34, 37) {
		t.Fatalf("uploaded image is %v, want the cells' 34x37", got)
	}
	alpha := func(x, y int) uint32 { _, _, _, a := sent.At(x, y).RGBA(); return a }
	if alpha(17, 0) != 0 || alpha(17, 36) != 0 {
		t.Errorf("top and bottom rows are drawn (alpha %d, %d), want the square left its 34 px", alpha(17, 0), alpha(17, 36))
	}
	if alpha(0, 18) == 0 || alpha(33, 18) == 0 || alpha(17, 2) == 0 || alpha(17, 34) == 0 {
		t.Error("the square does not span the cells' width and its own height")
	}
}
