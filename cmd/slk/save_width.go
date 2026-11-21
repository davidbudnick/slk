package main

import "strconv"

// saveWorkspaceWidth rewrites or appends a sidebar_width entry in
// [workspaces.<tomlKey>]. Mirrors saveWorkspaceTheme.
func saveWorkspaceWidth(configPath, tomlKey, teamID, teamName string, width int) error {
	return saveWorkspaceEntry(configPath, tomlKey, teamID, teamName, "sidebar_width", strconv.Itoa(width))
}
