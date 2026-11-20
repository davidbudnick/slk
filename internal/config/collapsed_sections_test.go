package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestResolveCollapsedSections(t *testing.T) {
	cases := []struct {
		name      string
		toml      string
		want      []string
		wantSaved bool
	}{
		{
			name:      "absent key is unsaved",
			toml:      "[workspaces.acme]\nteam_id = \"T01ABCDEF\"\n",
			wantSaved: false,
		},
		{
			name:      "empty list is saved",
			toml:      "[workspaces.acme]\nteam_id = \"T01ABCDEF\"\ncollapsed_sections = []\n",
			want:      []string{},
			wantSaved: true,
		},
		{
			name:      "names load in order",
			toml:      "[workspaces.acme]\nteam_id = \"T01ABCDEF\"\ncollapsed_sections = [\"Team\", \"Direct messages\"]\n",
			want:      []string{"Team", "Direct messages"},
			wantSaved: true,
		},
		{
			name:      "multi-line list loads",
			toml:      "[workspaces.acme]\nteam_id = \"T01ABCDEF\"\ncollapsed_sections = [\n  \"Team\",\n  \"Skip\",\n]\n",
			want:      []string{"Team", "Skip"},
			wantSaved: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.toml), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			got, saved := cfg.ResolveCollapsedSections("T01ABCDEF")
			if saved != tc.wantSaved {
				t.Fatalf("saved = %v, want %v", saved, tc.wantSaved)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("names = %q, want %q", got, tc.want)
			}
			if saved && got == nil {
				t.Error("a saved list resolved to nil, which reads as unsaved")
			}
		})
	}
}

func TestResolveCollapsedSectionsUnknownWorkspace(t *testing.T) {
	cfg := Config{Workspaces: map[string]Workspace{
		"acme": {TeamID: "T01ABCDEF", CollapsedSections: []string{"Team"}},
	}}
	if got, saved := cfg.ResolveCollapsedSections("T99OTHER"); saved || got != nil {
		t.Errorf("unknown workspace = %q, %v, want nil, false", got, saved)
	}
}

func TestResolveCollapsedSectionsCopies(t *testing.T) {
	cfg := Config{Workspaces: map[string]Workspace{
		"acme": {TeamID: "T01ABCDEF", CollapsedSections: []string{"Team"}},
	}}
	got, _ := cfg.ResolveCollapsedSections("T01ABCDEF")
	got[0] = "changed"
	if cfg.Workspaces["acme"].CollapsedSections[0] != "Team" {
		t.Error("the resolved list aliases the config's")
	}
}
