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

func TestSidebarIcon_InsetRoundedAvatarWithItsDotInAGap(t *testing.T) {
	red := imgcolor.NRGBA{R: 200, A: 255}
	green := imgcolor.NRGBA{G: 200, A: 255}
	centre := int(iconCanvas - badgeRadius - badgeEdgeSpace)
	inGap := centre - int(badgeRadius) - int(badgeGap)/2

	filled := sidebarIcon(solidAvatar(72, red), green, false).(*image.NRGBA)
	if got := filled.Bounds().Size(); got != image.Pt(iconCanvas, iconCanvas) {
		t.Fatalf("canvas is %v, want %d square", got, iconCanvas)
	}
	if got := filled.NRGBAAt(iconCanvas/2, iconCanvas/3); got != red {
		t.Errorf("avatar = %v, want the avatar's own %v", got, red)
	}
	if got := filled.NRGBAAt(iconCanvas/2, 2); got.A != 0 {
		t.Errorf("margin above the avatar = %v, want transparent so stacked avatars do not touch", got)
	}
	if got := filled.NRGBAAt(int(iconInset)+1, int(iconInset)+1); got.A != 0 {
		t.Errorf("avatar corner = %v, want it rounded off", got)
	}
	if got := filled.NRGBAAt(centre, centre); got != green {
		t.Errorf("dot centre = %v, want %v", got, green)
	}
	if got := filled.NRGBAAt(inGap, centre); got.A != 0 {
		t.Errorf("gap beside the dot = %v, want transparent", got)
	}

	hollow := sidebarIcon(solidAvatar(72, red), green, true).(*image.NRGBA)
	if got := hollow.NRGBAAt(centre, centre); got.A != 0 {
		t.Errorf("hollow dot centre = %v, want transparent", got)
	}
	if got := hollow.NRGBAAt(centre-int(badgeRadius)+2, centre); got != green {
		t.Errorf("hollow dot line = %v, want %v", got, green)
	}

	plain := sidebarIcon(solidAvatar(72, red), nil, false).(*image.NRGBA)
	if got := plain.NRGBAAt(centre-int(badgeRadius), centre-int(badgeRadius)); got != red {
		t.Errorf("with no dot the avatar's corner area = %v, want the avatar", got)
	}
}

func TestGetSidebar_OneImagePerPresence(t *testing.T) {
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
	if got := c.GetSidebar("U1", green, false); got != "" {
		t.Fatalf("sidebar avatar before the avatar loaded = %q, want empty", got)
	}
	c.PreloadSync("U1", srv.URL)
	afterPreload := sideCh.Len()
	online := c.GetSidebar("U1", green, false)
	if !strings.ContainsRune(online, '\U0010EEEE') || sideCh.Len() == afterPreload {
		t.Fatalf("first sidebar avatar = %q with %d new upload bytes, want placeholders and an upload", online, sideCh.Len()-afterPreload)
	}
	afterFirst := sideCh.Len()
	if again := c.GetSidebar("U1", green, false); again != online || sideCh.Len() != afterFirst {
		t.Errorf("second call re-rendered or re-uploaded")
	}
	away := c.GetSidebar("U1", green, true)
	plain := c.GetSidebar("U1", nil, false)
	if away == "" || plain == "" || away == online || plain == online || plain == away {
		t.Errorf("filled, hollow and no dot must each be their own image: %q, %q, %q", online, away, plain)
	}
	if plain == c.GetSmall("U1") {
		t.Error("the sidebar avatar with no dot is the reply-row avatar, want the inset one")
	}

	halfBlock := NewCache(imgpkg.NewFetcher(cache, http.DefaultClient), nil, false)
	halfBlock.PreloadSync("U1", srv.URL)
	if got := halfBlock.GetSidebar("U1", green, false); got != "" {
		t.Errorf("sidebar avatar off kitty = %q, want empty", got)
	}
}
