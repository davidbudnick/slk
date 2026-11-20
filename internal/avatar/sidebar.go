package avatar

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"golang.org/x/image/draw"

	imgpkg "github.com/gammons/slk/internal/image"
)

// iconCanvas is the side, in pixels, of the square a sidebar avatar is
// drawn on before kitty scales it to its cells.
const iconCanvas = 96

// The sidebar avatar's geometry on that canvas. The avatar is inset so
// avatars on adjacent rows do not touch, and its corners are rounded,
// as Slack's are. The presence dot sits on the bottom-right corner,
// half outside the avatar, inside a transparent gap.
const (
	iconInset      = 8.0
	iconCorner     = 14.0
	badgeRadius    = 16.0
	badgeGap       = 5.0
	badgeLine      = 7.0
	badgeEdgeSpace = 1.0
)

// GetSidebar returns the sidebar's avatar: SmallCols×SmallRows cells,
// inset and round-cornered, with a presence dot in dot's colour on its
// corner, a ring when hollow. A nil dot draws none. Empty if the avatar
// is not loaded yet or not on kitty.
func (c *Cache) GetSidebar(userID string, dot color.Color, hollow bool) string {
	if !c.useKitty {
		return ""
	}
	key := "avatar-" + userID + "-sidebar"
	if dot != nil {
		key += fmt.Sprintf("-%s-%t", hexRGB(dot), hollow)
	}
	c.mu.RLock()
	icon, src := c.sidebar[key], c.sources[userID]
	c.mu.RUnlock()
	if icon != "" || src == nil {
		return icon
	}
	return c.placeSidebar(key, sidebarIcon(src, dot, hollow), SmallCols)
}

// GetSidebarDefault returns the sidebar avatar of someone whose own is
// not loaded: a plain figure, as Slack gives someone with no photo,
// with the presence dot GetSidebar draws. Empty off kitty.
func (c *Cache) GetSidebarDefault(dot color.Color, hollow bool) string {
	if !c.useKitty {
		return ""
	}
	key := "avatar-default-sidebar"
	if dot != nil {
		key += fmt.Sprintf("-%s-%t", hexRGB(dot), hollow)
	}
	c.mu.RLock()
	icon := c.sidebar[key]
	c.mu.RUnlock()
	if icon != "" {
		return icon
	}
	return c.placeSidebar(key, sidebarIcon(defaultAvatar(), dot, hollow), SmallCols)
}

// The default avatar: a head and shoulders, centred, on a plain square.
const (
	defaultHeadY      = 37.0
	defaultHeadRadius = 16.0
	defaultBodyY      = 98.0
	defaultBodyRadius = 36.0
)

var (
	defaultAvatarFill   = color.NRGBA{R: 0x5c, G: 0x64, B: 0x73, A: 0xff}
	defaultAvatarFigure = color.NRGBA{R: 0xd9, G: 0xdd, B: 0xe3, A: 0xff}
)

// defaultAvatar draws the figure shown for someone with no avatar.
func defaultAvatar() image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, iconCanvas, iconCanvas))
	for y := 0; y < iconCanvas; y++ {
		for x := 0; x < iconCanvas; x++ {
			dx, fy := float64(x)+0.5-iconCanvas/2, float64(y)+0.5
			px := defaultAvatarFill
			if math.Hypot(dx, fy-defaultHeadY) <= defaultHeadRadius || math.Hypot(dx, fy-defaultBodyY) <= defaultBodyRadius {
				px = defaultAvatarFigure
			}
			dst.SetNRGBA(x, y, px)
		}
	}
	return dst
}

// placeSidebar uploads img under key and returns its placement, cols
// cells wide on one sidebar row, remembering it under key.
func (c *Cache) placeSidebar(key string, img image.Image, cols int) string {
	c.kitty.SetSource(key, img)
	out := c.kitty.RenderKey(key, image.Pt(cols, SmallRows))
	if out.OnFlush != nil {
		_ = out.OnFlush(imgpkg.KittyOutput)
	}
	placed := joinLines(out.Lines)
	c.mu.Lock()
	c.sidebar[key] = placed
	c.mu.Unlock()
	return placed
}

// sidebarIcon draws src as the sidebar shows it: inset on a transparent
// square with rounded corners, and, when dot is set, a presence dot on
// the bottom-right corner. The row's background shows through every
// transparent part: the margin, the gap around the dot, and a hollow
// dot's centre.
func sidebarIcon(src image.Image, dot color.Color, hollow bool) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, iconCanvas, iconCanvas))
	inner := image.Rect(int(iconInset), int(iconInset), iconCanvas-int(iconInset), iconCanvas-int(iconInset))
	draw.BiLinear.Scale(dst, inner, src, src.Bounds(), draw.Src, nil)
	for y := inner.Min.Y; y < inner.Max.Y; y++ {
		for x := inner.Min.X; x < inner.Max.X; x++ {
			if outsideRoundedCorner(float64(x)+0.5, float64(y)+0.5, iconInset, iconCanvas-iconInset) {
				dst.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
	if dot == nil {
		return dst
	}
	fill := color.NRGBAModel.Convert(dot).(color.NRGBA)
	centre := iconCanvas - badgeRadius - badgeEdgeSpace
	for y := int(centre - badgeRadius - badgeGap); y < iconCanvas; y++ {
		for x := int(centre - badgeRadius - badgeGap); x < iconCanvas; x++ {
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

// outsideRoundedCorner reports whether (x, y) falls outside the rounded
// corners of the square spanning lo..hi on both axes.
func outsideRoundedCorner(x, y, lo, hi float64) bool {
	return !inRoundRect(x, y, lo, lo, hi, hi, iconCorner)
}
