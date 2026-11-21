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

// Every icon is drawn in the avatar's square and centred there, so a
// column of icons, avatars and group squares lines up.
func TestLineIcon_EachKindIsCentredInTheAvatarsSquare(t *testing.T) {
	grey := imgcolor.NRGBA{R: 120, G: 124, B: 150, A: 255}
	none := imgcolor.NRGBA{}
	mid := iconCanvas / 2
	seen := map[string]bool{}
	for _, kind := range []string{IconChannels, IconDirectMessages, IconApps, IconHash, IconLock} {
		icon, ok := lineIcon(kind, grey).(*image.NRGBA)
		if !ok {
			t.Fatalf("%s: no drawing", kind)
		}
		if icon.Bounds().Size() != image.Pt(iconCanvas, iconCanvas) {
			t.Fatalf("%s: canvas is %v, want the avatar's %d square", kind, icon.Bounds().Size(), iconCanvas)
		}
		left := inked(icon, image.Rect(0, 0, mid, iconCanvas), none)
		right := inked(icon, image.Rect(mid, 0, iconCanvas, iconCanvas), none)
		if left == 0 || right == 0 || left*2 < right || right*2 < left {
			t.Errorf("%s: ink left %d, right %d of centre, want it centred", kind, left, right)
		}
		if edge := inked(icon, image.Rect(0, 0, iconCanvas, int(iconMargin)-6), none); edge != 0 {
			t.Errorf("%s: %d pixels drawn in the top margin, want it smaller than an avatar", kind, edge)
		}
		seen[string(icon.Pix)] = true
	}
	if len(seen) != 5 {
		t.Errorf("%d distinct drawings for 5 kinds", len(seen))
	}
	if lineIcon("stars", grey) != nil {
		t.Error("a kind with no drawing returned an image")
	}
}

func TestGetIcon_OneImagePerKindAndColour(t *testing.T) {
	cache, err := imgpkg.NewCache(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	saved := imgpkg.KittyOutput
	defer func() { imgpkg.KittyOutput = saved }()
	var sideCh bytes.Buffer
	imgpkg.KittyOutput = &sideCh
	grey := imgcolor.NRGBA{R: 120, G: 124, B: 150, A: 255}
	white := imgcolor.NRGBA{R: 240, G: 240, B: 240, A: 255}

	c := NewCache(imgpkg.NewFetcher(cache, http.DefaultClient), imgpkg.NewKittyRenderer(imgpkg.NewRegistry()), true)
	hash := c.GetIcon(IconHash, grey)
	if strings.Count(hash, string(imgpkg.PlaceholderRune)) != SmallCols || sideCh.Len() == 0 {
		t.Fatalf("hash icon = %q with %d upload bytes, want %d cells and an upload", hash, sideCh.Len(), SmallCols)
	}
	uploaded := sideCh.Len()
	if again := c.GetIcon(IconHash, grey); again != hash || sideCh.Len() != uploaded {
		t.Error("second call re-rendered or re-uploaded")
	}
	if lock, bright := c.GetIcon(IconLock, grey), c.GetIcon(IconHash, white); lock == "" || bright == "" || lock == hash || bright == hash {
		t.Errorf("another kind and another colour must each be their own image: %q, %q, %q", hash, lock, bright)
	}
	if got := c.GetIcon("stars", grey); got != "" {
		t.Errorf("icon of an unknown kind = %q, want empty", got)
	}

	halfBlock := NewCache(imgpkg.NewFetcher(cache, http.DefaultClient), nil, false)
	if got := halfBlock.GetIcon(IconHash, grey); got != "" {
		t.Errorf("icon off kitty = %q, want empty", got)
	}
}
