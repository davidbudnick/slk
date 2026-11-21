package avatar

import (
	"image"
	"image/color"
)

// The icons GetIcon draws: one for each built-in Slack section's
// header, and the two a channel row carries.
const (
	IconChannels       = "channels"
	IconDirectMessages = "direct_messages"
	IconApps           = "apps"
	IconHash           = "hash"
	IconLock           = "lock"
)

// The geometry of a section icon on the sidebar's iconCanvas square: a
// line drawing a little smaller than an avatar, as Slack's are.
const (
	iconLine   = 6.0
	iconMargin = 18.0
)

// GetIcon returns one of Slack's line icons in col, in a sidebar
// avatar's cells and centred as an avatar is, so a column of icons and
// avatars lines up. Empty for a kind it has no drawing for, or off
// kitty.
func (c *Cache) GetIcon(kind string, col color.Color) string {
	if !c.useKitty || col == nil {
		return ""
	}
	key := "icon-" + kind + "-" + hexRGB(col)
	c.mu.RLock()
	placed := c.sidebar[key]
	c.mu.RUnlock()
	if placed != "" {
		return placed
	}
	icon := lineIcon(kind, col)
	if icon == nil {
		return ""
	}
	return c.placeSidebar(key, icon, SmallCols)
}

// lineIcon draws kind's icon in col: a boxed hash for the channels
// section, a speech bubble for direct messages, a grid with a plus for
// apps, a hash for a channel and a padlock for a private one. Nil for
// any other kind.
func lineIcon(kind string, col color.Color) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, iconCanvas, iconCanvas))
	lo, hi := iconMargin, iconCanvas-iconMargin
	mid := float64(iconCanvas) / 2
	switch kind {
	case IconHash:
		for _, at := range []float64{mid - 11, mid + 11} {
			fillRoundRect(dst, at-iconLine/2, lo+4, at+iconLine/2, hi-4, 0, col)
			fillRoundRect(dst, lo+4, at-iconLine/2, hi-4, at+iconLine/2, 0, col)
		}
	case IconLock:
		const bodyTop = 44.0
		strokeRoundRect(dst, mid-15, lo+2, mid+15, hi, 15, iconLine, col)
		for y := int(bodyTop); y < iconCanvas; y++ {
			for x := 0; x < iconCanvas; x++ {
				dst.SetNRGBA(x, y, color.NRGBA{})
			}
		}
		strokeRoundRect(dst, mid-24, bodyTop, mid+24, hi+2, 8, iconLine, col)
	case IconChannels:
		strokeRoundRect(dst, lo, lo, hi, hi, 13, iconLine, col)
		for _, at := range []float64{mid - 10, mid + 10} {
			fillRoundRect(dst, at-iconLine/2, lo+13, at+iconLine/2, hi-13, 0, col)
			fillRoundRect(dst, lo+13, at-iconLine/2, hi-13, at+iconLine/2, 0, col)
		}
	case IconDirectMessages:
		bottom := hi - 12
		strokeRoundRect(dst, lo-2, lo, hi+2, bottom, 16, iconLine, col)
		for i := 0.0; i < 15; i++ {
			fillRoundRect(dst, lo+8, bottom+i-1, lo+8+(15-i), bottom+i, 0, col)
		}
	case IconApps:
		const cell, gap = 25.0, 10.0
		start := mid - cell - gap/2
		for row := 0.0; row < 2; row++ {
			for column := 0.0; column < 2; column++ {
				x, y := start+column*(cell+gap), start+row*(cell+gap)
				if row == 0 && column == 1 {
					fillRoundRect(dst, x, y+cell/2-iconLine/2, x+cell, y+cell/2+iconLine/2, 0, col)
					fillRoundRect(dst, x+cell/2-iconLine/2, y, x+cell/2+iconLine/2, y+cell, 0, col)
					continue
				}
				strokeRoundRect(dst, x, y, x+cell, y+cell, 6, iconLine-1, col)
			}
		}
	default:
		return nil
	}
	return dst
}

// strokeRoundRect draws the outline, width wide and inside the bounds,
// of the rectangle x0,y0..x1,y1 with its corners rounded to radius.
func strokeRoundRect(dst *image.NRGBA, x0, y0, x1, y1, radius, width float64, col color.Color) {
	line := color.NRGBAModel.Convert(col).(color.NRGBA)
	inner := radius - width
	if inner < 0 {
		inner = 0
	}
	for y := int(y0); y < int(y1)+1; y++ {
		for x := int(x0); x < int(x1)+1; x++ {
			covered := 0
			for sy := 0; sy < edgeSamples; sy++ {
				for sx := 0; sx < edgeSamples; sx++ {
					px := float64(x) + (float64(sx)+0.5)/edgeSamples
					py := float64(y) + (float64(sy)+0.5)/edgeSamples
					if inRoundRect(px, py, x0, y0, x1, y1, radius) &&
						!inRoundRect(px, py, x0+width, y0+width, x1-width, y1-width, inner) {
						covered++
					}
				}
			}
			if covered == 0 || !image.Pt(x, y).In(dst.Bounds()) {
				continue
			}
			px := line
			px.A = uint8(int(line.A) * covered / (edgeSamples * edgeSamples))
			dst.SetNRGBA(x, y, px)
		}
	}
}
