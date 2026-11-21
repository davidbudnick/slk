package sidebar

import (
	"slices"
	"testing"
)

func savedCollapseItems() []ChannelItem {
	return []ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
		{ID: "C2", Name: "eng", Type: "channel", Section: "Team"},
		{ID: "D1", Name: "alice", Type: "dm"},
	}
}

func slackSavedCollapseModel(sections ...SectionMeta) (Model, *fakeProvider) {
	provider := &fakeProvider{ready: true, sections: sections}
	m := New([]ChannelItem{
		{ID: "C1", Name: "general", Type: "channel", Section: "L1"},
		{ID: "C2", Name: "eng", Type: "channel", Section: "L2"},
		{ID: "D1", Name: "alice", Type: "dm", Section: "L3"},
	})
	m.SetSectionsProvider(provider)
	return m, provider
}

func TestSetSavedCollapsedSections_ConfigMode(t *testing.T) {
	m := New(savedCollapseItems())
	m.SetSavedCollapsedSections([]string{"Team"})

	if !m.IsCollapsed("Team") {
		t.Error("Team is saved as collapsed, got expanded")
	}
	if m.IsCollapsed("Channels") {
		t.Error("Channels is not in the saved list, so the default collapse must not apply")
	}
	if m.IsCollapsed("Direct Messages") {
		t.Error("Direct Messages is not in the saved list, got collapsed")
	}
}

func TestSetSavedCollapsedSections_EmptyListExpandsEverything(t *testing.T) {
	m := New(savedCollapseItems())
	m.SetStartCollapsed(true)
	m.SetSavedCollapsedSections([]string{})

	for _, name := range []string{"Team", "Channels", "Direct Messages", "Apps"} {
		if m.IsCollapsed(name) {
			t.Errorf("%s collapsed, want an empty saved list to win over start_collapsed and the defaults", name)
		}
	}
	if got := m.CollapsedSections(); got == nil || len(got) != 0 {
		t.Errorf("CollapsedSections() = %#v, want a non-nil empty list", got)
	}
}

func TestSetSavedCollapsedSections_NilRestoresDefaults(t *testing.T) {
	m := New(savedCollapseItems())
	m.SetSavedCollapsedSections([]string{"Team"})
	m.ToggleCollapse("Direct Messages")
	m.SetSavedCollapsedSections(nil)

	if !m.IsCollapsed("Channels") {
		t.Error("Channels should be back to its default, collapsed")
	}
	if m.IsCollapsed("Team") {
		t.Error("Team was only collapsed by the previous workspace's saved list")
	}
	if m.IsCollapsed("Direct Messages") {
		t.Error("the previous workspace's toggle leaked across the reset")
	}
	m.SetStartCollapsed(true)
	if !m.IsCollapsed("Team") {
		t.Error("with nothing saved, start_collapsed decides again")
	}
}

func TestSetSavedCollapsedSections_HidesRows(t *testing.T) {
	m := New(savedCollapseItems())
	m.SetSavedCollapsedSections([]string{"Direct Messages"})

	rows := map[string]bool{}
	for _, n := range m.nav {
		if n.kind == navChannel {
			rows[m.items[m.filtered[n.fi]].ID] = true
		}
	}
	if !rows["C1"] {
		t.Error("general sits in the expanded Channels section and should have a row")
	}
	if rows["D1"] {
		t.Error("alice is in a saved-collapsed section and should have no row")
	}
}

func TestToggleAfterSavedCollapsedSections(t *testing.T) {
	m := New(savedCollapseItems())
	m.SetSavedCollapsedSections([]string{"Team"})

	m.ToggleCollapse("Team")
	if m.IsCollapsed("Team") {
		t.Error("toggling a saved-collapsed section should expand it")
	}
	m.ToggleCollapse("Channels")
	if !m.IsCollapsed("Channels") {
		t.Error("toggling an expanded section should collapse it")
	}
	if got, want := m.CollapsedSections(), []string{"Channels"}; !slices.Equal(got, want) {
		t.Errorf("CollapsedSections() = %q, want %q", got, want)
	}
}

func TestCollapsedSections_DefaultsAndStartCollapsed(t *testing.T) {
	m := New(savedCollapseItems())
	if got, want := m.CollapsedSections(), []string{"Channels", "Apps"}; !slices.Equal(got, want) {
		t.Errorf("defaults: CollapsedSections() = %q, want %q (Apps has no rows but stays collapsed)", got, want)
	}

	m.SetStartCollapsed(true)
	m.ToggleCollapse("Team")
	if got, want := m.CollapsedSections(), []string{"Direct Messages", "Channels", "Apps"}; !slices.Equal(got, want) {
		t.Errorf("start_collapsed: CollapsedSections() = %q, want %q", got, want)
	}
}

func TestCollapsedSections_KeepsSavedNamesItCannotSee(t *testing.T) {
	m := New(savedCollapseItems())
	m.SetSavedCollapsedSections([]string{"Zed", "Archive", "Team"})

	m.ToggleCollapse("Direct Messages")
	if got, want := m.CollapsedSections(), []string{"Team", "Direct Messages", "Archive", "Zed"}; !slices.Equal(got, want) {
		t.Errorf("CollapsedSections() = %q, want %q", got, want)
	}
}

func TestSavedCollapsedSections_SlackModeMatchesByName(t *testing.T) {
	m, _ := slackSavedCollapseModel(
		SectionMeta{ID: "L1", Name: "Team", Type: "standard"},
		SectionMeta{ID: "L2", Name: "Skip", Type: "standard"},
		SectionMeta{ID: "L3", Name: "Direct messages", Type: "direct_messages"},
	)
	m.SetSavedCollapsedSections([]string{"Skip"})

	if m.IsCollapsed("L1") || !m.IsCollapsed("L2") || m.IsCollapsed("L3") {
		t.Errorf("collapsed = L1:%v L2:%v L3:%v, want only L2 (Skip)",
			m.IsCollapsed("L1"), m.IsCollapsed("L2"), m.IsCollapsed("L3"))
	}

	m.ToggleCollapse("L3")
	if got, want := m.CollapsedSections(), []string{"Skip", "Direct messages"}; !slices.Equal(got, want) {
		t.Errorf("CollapsedSections() = %q, want %q (names, in sidebar order)", got, want)
	}
}

func TestSavedCollapsedSections_SlackModeStartCollapsedSnapshot(t *testing.T) {
	m, _ := slackSavedCollapseModel(
		SectionMeta{ID: "L1", Name: "Team", Type: "standard"},
		SectionMeta{ID: "L2", Name: "Skip", Type: "standard"},
		SectionMeta{ID: "L3", Name: "Direct messages", Type: "direct_messages"},
	)
	m.SetStartCollapsed(true)
	m.ToggleCollapse("L1")

	if got, want := m.CollapsedSections(), []string{"Skip", "Direct messages"}; !slices.Equal(got, want) {
		t.Errorf("CollapsedSections() = %q, want %q", got, want)
	}
}

func TestSavedCollapsedSections_SlackModeSurvivesRename(t *testing.T) {
	m, provider := slackSavedCollapseModel(
		SectionMeta{ID: "L1", Name: "Team", Type: "standard"},
		SectionMeta{ID: "L2", Name: "Skip", Type: "standard"},
	)
	m.SetSavedCollapsedSections([]string{"Team"})
	if !m.IsCollapsed("L1") {
		t.Fatal("precondition: Team is saved as collapsed")
	}

	provider.sections = []SectionMeta{
		{ID: "L1", Name: "Squad", Type: "standard"},
		{ID: "L2", Name: "Skip", Type: "standard"},
	}
	if !m.IsCollapsed("L1") {
		t.Error("a rename mid-session expanded a collapsed section")
	}
	if got, want := m.CollapsedSections(), []string{"Squad", "Team"}; !slices.Equal(got, want) {
		t.Errorf("CollapsedSections() = %q, want %q (new name, old name kept)", got, want)
	}
}

func TestSavedCollapsedSections_AppliesOnceSlackSectionsLoad(t *testing.T) {
	provider := &fakeProvider{}
	m := New([]ChannelItem{{ID: "C1", Name: "general", Type: "channel", Section: "L1"}})
	m.SetSectionsProvider(provider)
	m.SetSavedCollapsedSections([]string{"Team"})
	if got, want := m.CollapsedSections(), []string{"Team"}; !slices.Equal(got, want) {
		t.Errorf("before load: CollapsedSections() = %q, want the saved name kept: %q", got, want)
	}

	provider.ready = true
	provider.sections = []SectionMeta{{ID: "L1", Name: "Team", Type: "standard"}}
	m.SetSectionsProvider(provider)
	if !m.IsCollapsed("L1") {
		t.Error("Team should be collapsed once Slack's sections are known")
	}
}

func TestSetSavedCollapsedSections_BumpsVersion(t *testing.T) {
	m := New(savedCollapseItems())
	before := m.Version()
	m.SetSavedCollapsedSections([]string{"Team"})
	if m.Version() == before {
		t.Error("the saved state changes the view, so the version must change")
	}
}
