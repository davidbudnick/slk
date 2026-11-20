package ui

// toggleSidebarSection expands or collapses the section whose header is
// under the sidebar cursor and saves the workspace's collapsed sections.
// False when the cursor is not on a header.
func (a *App) toggleSidebarSection() bool {
	if !a.sidebar.ToggleCollapseSelected() {
		return false
	}
	if a.settings != nil && a.activeTeamID != "" {
		a.settings.SaveCollapsedSections(a.activeTeamID, a.sidebar.CollapsedSections())
	}
	return true
}
