package demo

import (
	"image"
	"slices"
	"strings"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui"
)

func TestNewRejectsUnknownScenario(t *testing.T) {
	if _, err := New("nope", testNow); err == nil || !strings.Contains(err.Error(), "hero") {
		t.Fatalf("New(nope) err = %v, want the list of scenarios", err)
	}
}

func TestStartupMsgs(t *testing.T) {
	d := testDemo(t)
	msgs := d.StartupMsgs()
	if len(msgs) != 3 {
		t.Fatalf("got %d startup messages, want 3", len(msgs))
	}
	lumen, ok := msgs[0].(ui.WorkspaceReadyMsg)
	if !ok || !lumen.InitialActive || lumen.TeamID != teamLumen || lumen.LastChannelID != chGeneral ||
		lumen.Theme != "tokyo night" || lumen.UserID != uAlex || len(lumen.Channels) == 0 {
		t.Errorf("first = %#v", msgs[0])
	}
	if dw, ok := msgs[1].(ui.WorkspaceReadyMsg); !ok || dw.InitialActive || dw.TeamID != teamDriftwood {
		t.Errorf("second = %#v", msgs[1])
	}
	if c, ok := msgs[2].(ui.ConnectionStateMsg); !ok || c.State != 1 {
		t.Errorf("third = %#v, want connected", msgs[2])
	}
	if d.InitialTheme() != "tokyo night" {
		t.Errorf("InitialTheme = %q", d.InitialTheme())
	}
}

func TestInstallWiresAnApp(t *testing.T) {
	testDemo(t).Install(ui.NewApp()) // must not panic; cmd/slk's smoke test renders it
}

func TestImageContextDefaultsToHalfBlock(t *testing.T) {
	ctx := testDemo(t).imageContext()
	if ctx.Protocol != imgpkg.ProtoHalfBlock || ctx.CellPixels != image.Pt(8, 16) {
		t.Errorf("default image context = %v at %v, want half-block at 8x16", ctx.Protocol, ctx.CellPixels)
	}
	if ctx.Fetcher == nil || ctx.MaxRows != 12 || ctx.MaxCols != 60 {
		t.Errorf("image context = %+v", ctx)
	}
}

func TestUseImageProtocolReachesTheImageContext(t *testing.T) {
	d := testDemo(t)
	d.UseImageProtocol(imgpkg.ProtoSixel, image.Pt(11, 23))
	ctx := d.imageContext()
	if ctx.Protocol != imgpkg.ProtoSixel || ctx.CellPixels != image.Pt(11, 23) {
		t.Errorf("image context = %v at %v, want sixel at 11x23", ctx.Protocol, ctx.CellPixels)
	}
}

// namesIn is how the demo shows the user, a DM peer, a DM, the group DM,
// a channel author, a thread reply's author and a new post's author.
func namesIn(t *testing.T, d *Demo) []string {
	t.Helper()
	lumen := d.StartupMsgs()[0].(ui.WorkspaceReadyMsg)
	chans := map[string]string{}
	for _, c := range lumen.Channels {
		chans[c.ID] = c.Name
	}
	w := d.world
	msgs, _ := w.messages(dmSam)
	reply := w.replies(chEngineering, threadIn(t, w, chEngineering))[0]
	posted, _ := w.post(chGeneral, "", uMaya, "hi", false)
	return []string{lumen.UserNames[uAlex], lumen.UserNames[uSam], chans[dmSam], chans[gdmLaunch],
		msgs[0].UserName, lumen.UserNames[reply.UserID] + "/" + reply.UserName, posted.UserName}
}

func TestDisplayNamesUnlessFullNames(t *testing.T) {
	got := namesIn(t, testDemo(t))
	want := []string{"alex", "sam.okafor", "sam.okafor", "priya, sam.okafor, maya", "sam.okafor", "", "maya"}
	if r := strings.Split(got[5], "/"); r[0] != r[1] || strings.Contains(r[0], " ") {
		t.Errorf("by default the reply author = %q, want one display name", got[5])
	}
	if got[5] = ""; !slices.Equal(got, want) {
		t.Errorf("by default = %q, want display names %q", got, want)
	}
	d := testDemo(t)
	d.UseFullNames(true)
	got = namesIn(t, d)
	want = []string{"Alex Rivera", "Sam Okafor", "Sam Okafor", "Priya Shah, Sam Okafor, Maya Chen", "Sam Okafor", "", "Maya Chen"}
	if r := strings.Split(got[5], "/"); r[0] != r[1] || !strings.Contains(r[0], " ") {
		t.Errorf("with full names the reply author = %q, want one full name", got[5])
	}
	if got[5] = ""; !slices.Equal(got, want) {
		t.Errorf("with UseFullNames = %q, want full names %q", got, want)
	}
}
