package avatar

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"golang.org/x/image/draw"

	imgpkg "github.com/gammons/slk/internal/image"
)

// badgeCanvas is the side, in pixels, of the square a badged avatar is
// drawn on before kitty scales it to its cells.
const badgeCanvas = 96

// The badge's geometry on that canvas: the dot's radius, the transparent
// gap cut around it, and a hollow badge's line width.
const (
	badgeRadius = 21.0
	badgeGap    = 7.0
	badgeLine   = 8.0
)

// GetSmallBadged returns the sidebar avatar: GetSmall with a presence
// dot in dot's colour on its bottom-right corner, a ring when hollow. A
// nil dot is GetSmall. Empty if the avatar is not loaded yet or not on
// kitty.
func (c *Cache) GetSmallBadged(userID string, dot color.Color, hollow bool) string {
	if dot == nil {
		return c.GetSmall(userID)
	}
	if !c.useKitty {
		return ""
	}
	r, g, b, _ := dot.RGBA()
	key := fmt.Sprintf("avatar-%s-badge-%02x%02x%02x-%t", userID, r>>8, g>>8, b>>8, hollow)
	c.mu.RLock()
	badged, src := c.badged[key], c.sources[userID]
	c.mu.RUnlock()
	if badged != "" || src == nil {
		return badged
	}
	c.kitty.SetSource(key, withBadge(src, dot, hollow))
	out := c.kitty.RenderKey(key, image.Pt(SmallCols, SmallRows))
	if out.OnFlush != nil {
		_ = out.OnFlush(imgpkg.KittyOutput)
	}
	badged = joinLines(out.Lines)
	c.mu.Lock()
	c.badged[key] = badged
	c.mu.Unlock()
	return badged
}

// withBadge draws src on a square canvas with a presence dot on its
// bottom-right corner. The gap around the dot is transparent, so the
// row's background shows through it, as does a hollow badge's centre.
func withBadge(src image.Image, dot color.Color, hollow bool) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, badgeCanvas, badgeCanvas))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	fill := color.NRGBAModel.Convert(dot).(color.NRGBA)
	centre := badgeCanvas - badgeRadius
	for y := int(centre - badgeRadius - badgeGap); y < badgeCanvas; y++ {
		for x := int(centre - badgeRadius - badgeGap); x < badgeCanvas; x++ {
			d := math.Hypot(float64(x)+0.5-centre, float64(y)+0.5-centre)
			switch {
			case d > badgeRadius+badgeGap:
			case d > badgeRadius, hollow && d < badgeRadius-badgeLine:
				dst.SetNRGBA(x, y, color.NRGBA{})
			default:
				dst.SetNRGBA(x, y, fill)
			}
		}
	}
	return dst
}
