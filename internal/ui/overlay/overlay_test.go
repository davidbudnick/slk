package overlay_test

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/overlay"
)

// TestDimmedOverlayKeepsBackgroundKittyPlaceholders guards issue #18
// and what replaced its first fix.
//
// Kitty's unicode-placeholder protocol encodes an image's ID in each
// placeholder cell's SGR foreground color (R = byte 2, G = byte 1,
// B = byte 0 of the 24-bit ID). The original bug: DimmedOverlay's
// per-cell FG darken mutated those IDs, surfacing whatever image
// happened to own the darkened ID.
//
// The first fix blanked the cells, so every avatar, emoji and picture
// behind a modal vanished while it was open: opening the reaction
// picker emptied the pane it was opened over. A placement's FG is now
// left exactly as it was and only the cell's background is dimmed, so
// the image stays put.
func TestDimmedOverlayKeepsBackgroundKittyPlaceholders(t *testing.T) {
	const id uint32 = 0x112233
	r := byte((id >> 16) & 0xFF)
	g := byte((id >> 8) & 0xFF)
	b := byte(id & 0xFF)
	idFG := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)

	cell := func(col rune) string { return string(image.PlaceholderRune) + "\u0305" + string(col) }
	placeholders := idFG + cell('\u0305') + cell('\u030d') + cell('\u030e') + "\x1b[39m"

	const width, height = 12, 3
	bg := strings.Join([]string{
		"\x1b[38;2;200;200;200m" + strings.Repeat("a", width) + "\x1b[0m",
		placeholders + strings.Repeat(" ", width-3),
		strings.Repeat("b", width),
	}, "\n")

	// A narrow box in the middle leaves the placeholders, at the row's
	// left, outside it.
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Width(2).
		Height(1).
		Render("X")

	out := overlay.DimmedOverlay(width, height, bg, box, 0.5)
	rows := strings.Split(out, "\n")

	want := idFG + cell('\u0305') + cell('\u030d') + cell('\u030e')
	if !strings.Contains(rows[1], want) {
		t.Fatalf("placements behind the modal lost their cells or their image ID:\n%q\nwant it to contain %q", rows[1], want)
	}
	if strings.Contains(rows[0], "38;2;200;200;200") {
		t.Errorf("text behind the modal was not dimmed:\n%q", rows[0])
	}
}

// TestDimmedOverlayKeepsBoxKittyPlaceholders covers a placeholder rune
// that sits INSIDE the modal box itself (as the user-profile dialog's
// avatar does). DimmedOverlay's step 5 splices the box's own bytes back
// onto the output row verbatim after the dim-and-copy pass corrupts
// their SGR-bundled form (see the "Step 5" comment in overlay.go). This
// test pins that the placeholder rune and its original
// image-ID-as-RGB foreground survive byte-for-byte on a box row.
func TestDimmedOverlayKeepsBoxKittyPlaceholders(t *testing.T) {
	const id uint32 = 7
	r := byte((id >> 16) & 0xFF)
	g := byte((id >> 8) & 0xFF)
	b := byte(id & 0xFF)
	idFG := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	placeholder := idFG + string(image.PlaceholderRune) + "\x1b[39m"

	const width, height = 20, 6
	bg := strings.Join([]string{
		strings.Repeat("a", width),
		strings.Repeat("a", width),
		strings.Repeat("a", width),
		strings.Repeat("a", width),
		strings.Repeat("a", width),
		strings.Repeat("a", width),
	}, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Width(4).
		Render(placeholder)

	out := overlay.DimmedOverlay(width, height, bg, box, 0.5)

	if !strings.Contains(out, placeholder) {
		t.Fatalf("box's kitty placeholder was not preserved byte-for-byte in the composited output:\n%q\nwant to contain:\n%q", out, placeholder)
	}
}
