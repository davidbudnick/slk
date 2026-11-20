package avatar

import (
	"bytes"
	"image"
	imgcolor "image/color"
	imgpng "image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
)

// solidAvatar is a size×size image filled with c.
func solidAvatar(size int, c imgcolor.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestWithBadge_DotSitsInATransparentGap(t *testing.T) {
	red := imgcolor.NRGBA{R: 200, A: 255}
	green := imgcolor.NRGBA{G: 200, A: 255}
	centre := int(badgeCanvas - badgeRadius)
	inGap := centre - int(badgeRadius) - int(badgeGap)/2

	filled := withBadge(solidAvatar(72, red), green, false).(*image.NRGBA)
	if got := filled.Bounds().Size(); got != image.Pt(badgeCanvas, badgeCanvas) {
		t.Fatalf("canvas is %v, want %d square", got, badgeCanvas)
	}
	if got := filled.NRGBAAt(4, 4); got != red {
		t.Errorf("avatar corner = %v, want the avatar's own %v", got, red)
	}
	if got := filled.NRGBAAt(centre, centre); got != green {
		t.Errorf("dot centre = %v, want %v", got, green)
	}
	if got := filled.NRGBAAt(inGap, centre); got.A != 0 {
		t.Errorf("gap beside the dot = %v, want transparent", got)
	}

	hollow := withBadge(solidAvatar(72, red), green, true).(*image.NRGBA)
	if got := hollow.NRGBAAt(centre, centre); got.A != 0 {
		t.Errorf("hollow badge centre = %v, want transparent", got)
	}
	if got := hollow.NRGBAAt(centre-int(badgeRadius)+2, centre); got != green {
		t.Errorf("hollow badge line = %v, want %v", got, green)
	}
}

func TestGetSmallBadged_OneImagePerBadge(t *testing.T) {
	t.Setenv("TMUX", "")
	var buf bytes.Buffer
	imgpng.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 32, 32)))
	pngBytes := buf.Bytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	defer srv.Close()
	cache, err := imgpkg.NewCache(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	saved := imgpkg.KittyOutput
	defer func() { imgpkg.KittyOutput = saved }()
	var sideCh bytes.Buffer
	imgpkg.KittyOutput = &sideCh

	green := imgcolor.NRGBA{G: 200, A: 255}
	c := NewCache(imgpkg.NewFetcher(cache, http.DefaultClient), imgpkg.NewKittyRenderer(imgpkg.NewRegistry()), true)
	if got := c.GetSmallBadged("U1", green, false); got != "" {
		t.Fatalf("badged avatar before the avatar loaded = %q, want empty", got)
	}
	c.PreloadSync("U1", srv.URL)
	afterPreload := sideCh.Len()
	online := c.GetSmallBadged("U1", green, false)
	if !strings.ContainsRune(online, '\U0010EEEE') || sideCh.Len() == afterPreload {
		t.Fatalf("first badged avatar = %q with %d new upload bytes, want placeholders and an upload", online, sideCh.Len()-afterPreload)
	}
	afterFirst := sideCh.Len()
	if again := c.GetSmallBadged("U1", green, false); again != online || sideCh.Len() != afterFirst {
		t.Errorf("second call re-rendered or re-uploaded")
	}
	if away := c.GetSmallBadged("U1", green, true); away == "" || away == online {
		t.Errorf("hollow badge = %q, want its own image, not the filled one's", away)
	}
	if plain := c.GetSmallBadged("U1", nil, false); plain == "" || plain != c.GetSmall("U1") {
		t.Errorf("no badge = %q, want the plain small avatar", plain)
	}

	halfBlock := NewCache(imgpkg.NewFetcher(cache, http.DefaultClient), nil, false)
	halfBlock.PreloadSync("U1", srv.URL)
	if got := halfBlock.GetSmallBadged("U1", green, false); got != "" {
		t.Errorf("badged avatar off kitty = %q, want empty", got)
	}
}
