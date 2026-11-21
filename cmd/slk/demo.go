package main

import (
	"context"
	"fmt"
	"image"
	"os"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/pelletier/go-toml/v2"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/demo"
	emojiwidth "github.com/gammons/slk/internal/emoji"
	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/styles"
)

// runDemo runs slk against the made-up workspaces in internal/demo. It is
// the recording mode behind the hidden --demo flag: no config, tokens,
// cache, network or config writes. See
// docs/superpowers/specs/2026-09-27-demo-mode-design.md.
// SLK_DEMO_TRANSPARENT=1 turns on the transparent background, which a
// user sets as [appearance] transparent_background. SLK_DEMO_LOOK names a
// TOML file whose [appearance] and [sidebar] settings the demo takes; a
// theme set there replaces every workspace's own.
func runDemo(scenario string) error {
	d, err := demo.New(scenario, time.Now())
	if err != nil {
		return err
	}
	proto, err := demoImageProtocol(os.Getenv("SLK_DEMO_IMAGES"))
	if err != nil {
		return err
	}
	settings, err := demoSettings(os.Getenv("SLK_DEMO_LOOK"))
	if err != nil {
		return err
	}
	if settings.Theme != "" {
		d.UseTheme(settings.Theme)
	}
	styles.SetTransparentBackground(os.Getenv("SLK_DEMO_TRANSPARENT") == "1")
	styles.Apply(d.InitialTheme(), core.Theme{})
	emojiwidth.SetImageMode(false, 2)

	var opts []tea.ProgramOption
	var frames *imgpkg.SixelFrameStore
	if proto == imgpkg.ProtoSixel {
		// Sixel is sized in pixels. ttyd (under VHS) reports no pixel
		// size, so the tapes pass COLORTERM_CELL_WIDTH/HEIGHT.
		pxW, pxH := imgpkg.CellPixels(int(os.Stdout.Fd()))
		imgpkg.SetCellPixels(pxW, pxH)
		d.UseImageProtocol(proto, image.Pt(pxW, pxH))
		// The same frame-correlated writer run() uses: sixel is painted
		// after each frame's text, in the frame it belongs to.
		frames = imgpkg.NewSixelFrameStore()
		opts = append(opts, tea.WithOutput(imgpkg.NewFrameOutput(os.Stdout, frames)))
	}
	app := newDemoApp(d, settings)
	if frames != nil {
		app.SetSixelFrameStore(frames)
	}

	p := tea.NewProgram(app, opts...)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.Run(ctx, p.Send)
	}()
	// Send blocks until the program's event loop is running, so the
	// workspaces "connect" from their own goroutine, as in run().
	go func() {
		for _, msg := range d.StartupMsgs() {
			p.Send(msg)
		}
	}()
	_, err = p.Run()
	cancel()
	wg.Wait()
	return err
}

// demoImageProtocol reads SLK_DEMO_IMAGES. Half-block, the default,
// works in any terminal; sixel is for recording with a VHS that captures
// sixel (docs/assets/demo/settings.tape).
func demoImageProtocol(value string) (imgpkg.Protocol, error) {
	switch value {
	case "", "halfblock":
		return imgpkg.ProtoHalfBlock, nil
	case "sixel":
		return imgpkg.ProtoSixel, nil
	}
	return imgpkg.ProtoOff, fmt.Errorf("SLK_DEMO_IMAGES=%q: want halfblock or sixel", value)
}

// demoSettings is the demo's look. With path empty it is the demo's own;
// otherwise the [appearance] and [sidebar] settings of the TOML file at
// path (SLK_DEMO_LOOK), so a recording can show an opt-in look. Only that
// file is read: never the user's config.
func demoSettings(path string) (uiSettings, error) {
	s := uiSettings{
		TypingIndicators:     true,
		HideDeactivatedUsers: true,
		MouseWheelLines:      3,
		ColoredUsernames:     true,
		ClickOpensThread:     true,
	}
	if path != "" {
		cfg, err := config.Load(path)
		if err != nil {
			return uiSettings{}, fmt.Errorf("SLK_DEMO_LOOK: %w", err)
		}
		theme, err := demoTheme(path)
		if err != nil {
			return uiSettings{}, err
		}
		s = uiSettingsFromConfig(cfg)
		s.Theme, s.ThemeOverrides = theme, core.Theme{}
	}
	s.TimestampFormat = demo.TimestampFormat
	return s, nil
}

// demoTheme is the built-in theme the look file at path names in
// [appearance] theme, spelled as styles.ThemeNames has it, or "" when the
// file names none and each workspace keeps its own.
func demoTheme(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("SLK_DEMO_LOOK: %w", err)
	}
	var look struct {
		Appearance struct {
			Theme string `toml:"theme"`
		} `toml:"appearance"`
	}
	if err := toml.Unmarshal(data, &look); err != nil {
		return "", fmt.Errorf("SLK_DEMO_LOOK: %w", err)
	}
	if look.Appearance.Theme == "" {
		return "", nil
	}
	for _, name := range styles.ThemeNames() {
		if strings.EqualFold(name, look.Appearance.Theme) {
			return name, nil
		}
	}
	return "", fmt.Errorf("SLK_DEMO_LOOK: unknown theme %q", look.Appearance.Theme)
}

// newDemoApp builds the demo's App with settings. It takes no token store
// or cache handle, so demo mode cannot reach the user's real data.
func newDemoApp(d *demo.Demo, settings uiSettings) *ui.App {
	app := ui.NewApp()
	applyUISettings(app, settings)
	d.Install(app)
	return app
}
