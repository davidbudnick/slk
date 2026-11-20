package avatar

import (
	"bytes"
	"image"
	imgcolor "image/color"
	"net/http"
	"strings"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
)

// inked counts the pixels of img inside r that are neither transparent
// nor fill: the label's.
func inked(img *image.NRGBA, r image.Rectangle, fill imgcolor.NRGBA) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if px := img.NRGBAAt(x, y); px.A != 0 && px != fill {
				n++
			}
		}
	}
	return n
}

func TestCountPill_LeavesAGapAboveAndBelow(t *testing.T) {
	blue := imgcolor.NRGBA{B: 200, A: 255}
	dark := imgcolor.NRGBA{R: 10, G: 10, B: 10, A: 255}

	one := countPill("2", blue, dark).(*image.NRGBA)
	if got := one.Bounds().Size(); got != image.Pt(2*countCell, iconCanvas) {
		t.Fatalf("one-digit canvas is %v, want two cells", got)
	}
	mid := iconCanvas / 2
	for _, y := range []int{0, int(pillInsetY) - 2, iconCanvas - int(pillInsetY) + 1, iconCanvas - 1} {
		if got := one.NRGBAAt(one.Bounds().Dx()/2, y); got.A != 0 {
			t.Errorf("row %d = %v, want transparent so stacked pills do not touch", y, got)
		}
	}
	if got := one.NRGBAAt(int(pillInsetX)+6, mid); got != blue {
		t.Errorf("pill's left side = %v, want the fill %v", got, blue)
	}
	if got := one.NRGBAAt(int(pillInsetX)+1, int(pillInsetY)+1); got.A != 0 {
		t.Errorf("pill's corner = %v, want it rounded off", got)
	}
	if n := inked(one, one.Bounds(), blue); n == 0 {
		t.Error("the pill has no digit drawn on it")
	}

	wide := countPill("99+", blue, dark).(*image.NRGBA)
	if got := wide.Bounds().Size(); got != image.Pt(4*countCell, iconCanvas) {
		t.Fatalf("99+ canvas is %v, want four cells", got)
	}
	for name, img := range map[string]*image.NRGBA{"2": one, "99+": wide} {
		w := img.Bounds().Dx()
		if got := img.NRGBAAt(w-int(pillInsetX)-4, mid); got != blue {
			t.Errorf("%s: pill does not end at the right of its cells: %v", name, got)
		}
		if got := img.NRGBAAt(w-2, mid); got.A != 0 {
			t.Errorf("%s: right margin = %v, want transparent", name, got)
		}
	}
	if got := wide.NRGBAAt(2, mid); got.A != 0 {
		t.Errorf("99+: left margin = %v, want transparent", got)
	}
}

func TestCountTile_IsTheAvatarsSquareWithTheCountCentred(t *testing.T) {
	grey := imgcolor.NRGBA{R: 120, G: 120, B: 120, A: 255}
	dark := imgcolor.NRGBA{R: 10, G: 10, B: 10, A: 255}
	red := imgcolor.NRGBA{R: 200, A: 255}

	tile := countTile("5", grey, dark).(*image.NRGBA)
	avatar := sidebarIcon(solidAvatar(72, red), nil, false).(*image.NRGBA)
	if tile.Bounds() != avatar.Bounds() {
		t.Fatalf("tile is %v, want the avatar's %v", tile.Bounds(), avatar.Bounds())
	}
	mid := iconCanvas / 2
	for _, p := range []image.Point{{mid, 2}, {2, mid}, {int(iconInset) + 1, int(iconInset) + 1}, {int(iconInset) + 3, mid}, {mid, int(iconInset) + 3}} {
		if got, want := tile.NRGBAAt(p.X, p.Y).A != 0, avatar.NRGBAAt(p.X, p.Y).A != 0; got != want {
			t.Errorf("at %v the tile is drawn: %v, the avatar: %v; want the same square", p, got, want)
		}
	}
	left := inked(tile, image.Rect(0, 0, mid, iconCanvas), grey)
	right := inked(tile, image.Rect(mid, 0, iconCanvas, iconCanvas), grey)
	if left == 0 || right == 0 {
		t.Fatalf("digit ink left %d, right %d of centre, want it across the middle", left, right)
	}
	top := inked(tile, image.Rect(0, 0, iconCanvas, mid), grey)
	bottom := inked(tile, image.Rect(0, mid, iconCanvas, iconCanvas), grey)
	if top*3 < bottom || bottom*3 < top {
		t.Errorf("digit ink above %d, below %d the middle, want it centred", top, bottom)
	}
}

func TestGetCount_OneImagePerLabelColoursAndShape(t *testing.T) {
	cache, err := imgpkg.NewCache(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	saved := imgpkg.KittyOutput
	defer func() { imgpkg.KittyOutput = saved }()
	var sideCh bytes.Buffer
	imgpkg.KittyOutput = &sideCh
	blue := imgcolor.NRGBA{B: 200, A: 255}
	dark := imgcolor.NRGBA{R: 10, G: 10, B: 10, A: 255}
	cells := func(s string) int { return strings.Count(s, string(imgpkg.PlaceholderRune)) }

	c := NewCache(imgpkg.NewFetcher(cache, http.DefaultClient), imgpkg.NewKittyRenderer(imgpkg.NewRegistry()), true)
	pill := c.GetCount("2", blue, dark, false)
	if cells(pill) != 2 || sideCh.Len() == 0 {
		t.Fatalf("pill = %q with %d upload bytes, want two cells and an upload", pill, sideCh.Len())
	}
	uploaded := sideCh.Len()
	if again := c.GetCount("2", blue, dark, false); again != pill || sideCh.Len() != uploaded {
		t.Error("second call re-rendered or re-uploaded")
	}
	if got := cells(c.GetCount("99+", blue, dark, false)); got != 4 {
		t.Errorf("99+ pill is %d cells, want 4", got)
	}
	tile := c.GetCount("2", blue, dark, true)
	if cells(tile) != SmallCols {
		t.Errorf("tile is %d cells, want an avatar's %d", cells(tile), SmallCols)
	}
	other := c.GetCount("2", dark, blue, false)
	if tile == pill || other == pill || other == "" {
		t.Errorf("pill, tile and other colours must each be their own image: %q, %q, %q", pill, tile, other)
	}
	if got := c.GetCount("2", nil, dark, false); got != "" {
		t.Errorf("count with no fill = %q, want empty", got)
	}

	halfBlock := NewCache(imgpkg.NewFetcher(cache, http.DefaultClient), nil, false)
	if got := halfBlock.GetCount("2", blue, dark, false); got != "" {
		t.Errorf("count off kitty = %q, want empty", got)
	}
}
