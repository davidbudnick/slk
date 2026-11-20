package ui

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/sidebar"
)

type collapsedSave struct {
	teamID string
	names  []string
}

func collapseTestApp(t *testing.T, teamID string) (*App, *[]collapsedSave) {
	t.Helper()
	a := newTestApp(t,
		withSize(160, 40),
		withChannels(
			sidebar.ChannelItem{ID: "C1", Name: "general", Type: "channel"},
			sidebar.ChannelItem{ID: "C2", Name: "eng", Type: "channel", Section: "Team"},
			sidebar.ChannelItem{ID: "D1", Name: "alice", Type: "dm"},
		),
		withActiveTeam(teamID),
	)
	a.sidebarVisible = true
	a.focusedPanel = PanelSidebar
	saves := &[]collapsedSave{}
	a.setCollapsedSectionsSaverForTest(func(teamID string, names []string) {
		*saves = append(*saves, collapsedSave{teamID: teamID, names: names})
	})
	return a, saves
}

func selectSidebarHeader(t *testing.T, a *App, want string) {
	t.Helper()
	a.sidebar.GoToTop()
	for range 50 {
		if name, ok := a.sidebar.IsSectionHeaderSelected(); ok && name == want {
			return
		}
		a.sidebar.MoveDown()
	}
	t.Fatalf("precondition: no %q section header is reachable", want)
}

func TestToggleSectionSavesCollapsedSections(t *testing.T) {
	presses := map[string]tea.Msg{
		"space": keyPress(' '),
		"enter": keyCode(tea.KeyEnter),
	}
	for name, press := range presses {
		t.Run(name, func(t *testing.T) {
			a, saves := collapseTestApp(t, "T1")
			selectSidebarHeader(t, a, "Team")

			_, _ = a.Update(press)
			if !a.sidebar.IsCollapsed("Team") {
				t.Fatal("Team should be collapsed after the press")
			}
			if len(*saves) != 1 {
				t.Fatalf("saver called %d times, want 1", len(*saves))
			}
			got := (*saves)[0]
			if got.teamID != "T1" {
				t.Errorf("saved for team %q, want T1", got.teamID)
			}
			if want := []string{"Team", "Channels", "Apps"}; !slices.Equal(got.names, want) {
				t.Errorf("saved %q, want %q", got.names, want)
			}

			_, _ = a.Update(press)
			if len(*saves) != 2 {
				t.Fatalf("saver called %d times after expanding again, want 2", len(*saves))
			}
			if want := []string{"Channels", "Apps"}; !slices.Equal((*saves)[1].names, want) {
				t.Errorf("saved %q after expanding, want %q", (*saves)[1].names, want)
			}
		})
	}
}

func TestClickOnSectionHeaderSavesCollapsedSections(t *testing.T) {
	a, saves := collapseTestApp(t, "T1")
	_ = a.View()

	headerY := -1
	for y := range 20 {
		a.sidebar.ClickAt(y)
		if name, ok := a.sidebar.IsSectionHeaderSelected(); ok && name == "Team" {
			headerY = y
			break
		}
	}
	if headerY < 0 {
		t.Fatal("precondition: no row of the sidebar is the Team header")
	}

	_ = reduceMouseClick(a, tea.MouseClickMsg{
		Button: tea.MouseLeft, X: a.layout.RailWidth() + 1, Y: headerY + 1,
	})
	if !a.sidebar.IsCollapsed("Team") {
		t.Fatal("clicking the Team header should collapse it")
	}
	if len(*saves) != 1 {
		t.Fatalf("saver called %d times, want 1", len(*saves))
	}
	if want := []string{"Team", "Channels", "Apps"}; !slices.Equal((*saves)[0].names, want) {
		t.Errorf("saved %q, want %q", (*saves)[0].names, want)
	}
}

func TestToggleSectionSavesNothingOffAHeader(t *testing.T) {
	a, saves := collapseTestApp(t, "T1")
	a.sidebar.GoToTop()

	_, _ = a.Update(keyPress(' '))
	if len(*saves) != 0 {
		t.Errorf("saver called %d times with the cursor on the Threads row, want 0", len(*saves))
	}
}

func TestToggleSectionWithoutWorkspaceStillToggles(t *testing.T) {
	a, saves := collapseTestApp(t, "")
	selectSidebarHeader(t, a, "Team")

	_, _ = a.Update(keyPress(' '))
	if !a.sidebar.IsCollapsed("Team") {
		t.Error("the section should still collapse with no workspace showing")
	}
	if len(*saves) != 0 {
		t.Errorf("saver called %d times with no workspace to save for, want 0", len(*saves))
	}
}

func TestToggleSectionWithoutSettingsService(t *testing.T) {
	a := newTestApp(t, withChannels(
		sidebar.ChannelItem{ID: "C2", Name: "eng", Type: "channel", Section: "Team"},
	), withActiveTeam("T1"))
	a.focusedPanel = PanelSidebar
	a.settings = nil
	selectSidebarHeader(t, a, "Team")

	_, _ = a.Update(keyPress(' '))
	if !a.sidebar.IsCollapsed("Team") {
		t.Error("the section should collapse with no settings service wired")
	}
}

func TestWorkspaceReadyRestoresCollapsedSections(t *testing.T) {
	a := newTestApp(t)
	_, _ = a.Update(WorkspaceReadyMsg{
		TeamID:            "T1",
		TeamName:          "One",
		InitialActive:     true,
		CollapsedSections: []string{"Team"},
		Channels: []sidebar.ChannelItem{
			{ID: "C1", Name: "general", Type: "channel"},
			{ID: "C2", Name: "eng", Type: "channel", Section: "Team"},
		},
	})

	if !a.sidebar.IsCollapsed("Team") {
		t.Error("Team is saved as collapsed and should start that way")
	}
	if a.sidebar.IsCollapsed("Channels") {
		t.Error("Channels is not in the saved list, so its default collapse must not apply")
	}
}

func TestWorkspaceReadyWithNothingSavedKeepsDefaults(t *testing.T) {
	a := newTestApp(t)
	_, _ = a.Update(WorkspaceReadyMsg{
		TeamID:        "T1",
		TeamName:      "One",
		InitialActive: true,
		Channels: []sidebar.ChannelItem{
			{ID: "C1", Name: "general", Type: "channel"},
			{ID: "C2", Name: "eng", Type: "channel", Section: "Team"},
		},
	})

	if !a.sidebar.IsCollapsed("Channels") {
		t.Error("with nothing saved, Channels keeps its default: collapsed")
	}
	if a.sidebar.IsCollapsed("Team") {
		t.Error("with nothing saved, a custom section starts expanded")
	}
}

func TestWorkspaceSwitchRestoresCollapsedSections(t *testing.T) {
	a, _ := collapseTestApp(t, "T1")
	selectSidebarHeader(t, a, "Direct Messages")
	_, _ = a.Update(keyPress(' '))
	if !a.sidebar.IsCollapsed("Direct Messages") {
		t.Fatal("precondition: Direct Messages collapsed in the first workspace")
	}

	channels := []sidebar.ChannelItem{
		{ID: "C9", Name: "general", Type: "channel"},
		{ID: "C8", Name: "eng", Type: "channel", Section: "Team"},
		{ID: "D9", Name: "bob", Type: "dm"},
	}
	_, _ = a.Update(WorkspaceSwitchedMsg{
		TeamID: "T2", TeamName: "Two", UserID: "U2",
		CollapsedSections: []string{"Team"},
		Channels:          channels,
	})
	if !a.sidebar.IsCollapsed("Team") || a.sidebar.IsCollapsed("Direct Messages") || a.sidebar.IsCollapsed("Channels") {
		t.Errorf("after switching to a workspace that saved [Team]: Team=%v DMs=%v Channels=%v",
			a.sidebar.IsCollapsed("Team"), a.sidebar.IsCollapsed("Direct Messages"), a.sidebar.IsCollapsed("Channels"))
	}

	_, _ = a.Update(WorkspaceSwitchedMsg{
		TeamID: "T3", TeamName: "Three", UserID: "U3",
		Channels: channels,
	})
	if a.sidebar.IsCollapsed("Team") || a.sidebar.IsCollapsed("Direct Messages") || !a.sidebar.IsCollapsed("Channels") {
		t.Errorf("after switching to a workspace with nothing saved, want the defaults: Team=%v DMs=%v Channels=%v",
			a.sidebar.IsCollapsed("Team"), a.sidebar.IsCollapsed("Direct Messages"), a.sidebar.IsCollapsed("Channels"))
	}
}
