package avatar

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The geometry of a count drawn as an image. Its canvas is iconCanvas
// tall and countCell wide for each cell. A pill is inset above and
// below, so the pills of adjacent rows do not touch, and ends at the
// right of its cells, so pills of different widths line up.
const (
	countCell     = iconCanvas / 2
	countTextSize = 58.0
	pillInsetX    = 6.0
	pillInsetY    = 16.0
	pillPadX      = 24.0
	pillMinWidth  = 84.0
	edgeSamples   = 3
)

// countFont is the face counts are drawn in.
var countFont = sync.OnceValue(func() font.Face {
	parsed, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: countTextSize, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil
	}
	return face
})

// countDrawing serializes drawing with countFont: a font.Face is not
// safe for concurrent use.
var countDrawing sync.Mutex

// GetCount returns label drawn as an image in text on fill: the square
// a group DM has for an avatar when tile is set, else an unread badge's
// pill, one cell wider than label. Empty off kitty.
func (c *Cache) GetCount(label string, fill, text color.Color, tile bool) string {
	if !c.useKitty || label == "" || fill == nil || text == nil || countFont() == nil {
		return ""
	}
	key := fmt.Sprintf("count-%s-%s-%s-%t", label, hexRGB(fill), hexRGB(text), tile)
	c.mu.RLock()
	placed := c.sidebar[key]
	c.mu.RUnlock()
	if placed != "" {
		return placed
	}
	countDrawing.Lock()
	defer countDrawing.Unlock()
	if tile {
		return c.placeSidebar(key, countTile(label, fill, text), SmallCols)
	}
	return c.placeSidebar(key, countPill(label, fill, text), pillCols(label))
}

// pillCols is the width in cells of label's pill.
func pillCols(label string) int {
	return len(label) + 1
}

// countPill draws label as Slack's unread badge: a pill in fill, as
// wide as label needs, at the right of pillCols cells.
func countPill(label string, fill, text color.Color) image.Image {
	width := float64(pillCols(label) * countCell)
	dst := image.NewNRGBA(image.Rect(0, 0, int(width), iconCanvas))
	right := width - pillInsetX
	left := right - math.Min(math.Max(pillMinWidth, inkWidth(label)+2*pillPadX), width-2*pillInsetX)
	fillRoundRect(dst, left, pillInsetY, right, iconCanvas-pillInsetY, iconCanvas/2-pillInsetY, fill)
	drawCount(dst, label, text, (left+right)/2)
	return dst
}

// countTile draws label on a square with a sidebar avatar's inset and
// corners, as Slack draws the number of people in a group DM.
func countTile(label string, fill, text color.Color) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, iconCanvas, iconCanvas))
	fillRoundRect(dst, iconInset, iconInset, iconCanvas-iconInset, iconCanvas-iconInset, iconCorner, fill)
	drawCount(dst, label, text, iconCanvas/2)
	return dst
}

// inkWidth is the width of label's drawn pixels.
func inkWidth(label string) float64 {
	bounds, _ := font.BoundString(countFont(), label)
	return float64(bounds.Max.X-bounds.Min.X) / 64
}

// drawCount draws label in col with its ink centred on x, and a digit's
// height centred on the canvas, so every label sits on one baseline.
func drawCount(dst *image.NRGBA, label string, col color.Color, x float64) {
	face := countFont()
	ink, _ := font.BoundString(face, label)
	digit, _ := font.BoundString(face, "0")
	centreY := fixed.I(dst.Bounds().Dy() / 2)
	d := font.Drawer{Dst: dst, Src: image.NewUniform(col), Face: face}
	d.Dot = fixed.Point26_6{
		X: fixed.Int26_6(x*64) - (ink.Min.X+ink.Max.X)/2,
		Y: centreY - (digit.Min.Y+digit.Max.Y)/2,
	}
	d.DrawString(label)
}

// fillRoundRect fills the rectangle x0,y0..x1,y1, its corners rounded
// to radius, with col. A pixel on the edge gets the share of col its
// covered part earns, so the edge is smooth.
func fillRoundRect(dst *image.NRGBA, x0, y0, x1, y1, radius float64, col color.Color) {
	fill := color.NRGBAModel.Convert(col).(color.NRGBA)
	for y := int(y0); y < int(math.Ceil(y1)); y++ {
		for x := int(x0); x < int(math.Ceil(x1)); x++ {
			covered := 0
			for sy := 0; sy < edgeSamples; sy++ {
				for sx := 0; sx < edgeSamples; sx++ {
					px := float64(x) + (float64(sx)+0.5)/edgeSamples
					py := float64(y) + (float64(sy)+0.5)/edgeSamples
					if inRoundRect(px, py, x0, y0, x1, y1, radius) {
						covered++
					}
				}
			}
			if covered == 0 {
				continue
			}
			px := fill
			px.A = uint8(int(fill.A) * covered / (edgeSamples * edgeSamples))
			dst.SetNRGBA(x, y, px)
		}
	}
}

// inRoundRect reports whether (x, y) is inside the rectangle
// x0,y0..x1,y1 with its corners rounded to radius.
func inRoundRect(x, y, x0, y0, x1, y1, radius float64) bool {
	if x < x0 || x >= x1 || y < y0 || y >= y1 {
		return false
	}
	cx := math.Max(x0+radius, math.Min(x, x1-radius))
	cy := math.Max(y0+radius, math.Min(y, y1-radius))
	return math.Hypot(x-cx, y-cy) <= radius
}

// hexRGB is col as rrggbb, for a cache key.
func hexRGB(col color.Color) string {
	r, g, b, _ := col.RGBA()
	return fmt.Sprintf("%02x%02x%02x", r>>8, g>>8, b>>8)
}
