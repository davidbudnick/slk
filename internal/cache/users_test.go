package cache

import (
	"testing"
)

func TestUpsertAndGetUser(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	u := User{
		ID:          "U123",
		WorkspaceID: "T1",
		Name:        "alice",
		DisplayName: "Alice Smith",
		Presence:    "active",
	}

	if err := db.UpsertUser(u); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetUser("U123")
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Alice Smith" {
		t.Errorf("expected 'Alice Smith', got %q", got.DisplayName)
	}
	if got.Presence != "active" {
		t.Errorf("expected 'active', got %q", got.Presence)
	}
}

func TestUpsertUserPreservesIsExternal(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	u := User{
		ID:          "U_EXT",
		WorkspaceID: "T1",
		Name:        "ext.user",
		DisplayName: "External User",
		IsExternal:  true,
	}
	if err := db.UpsertUser(u); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetUser("U_EXT")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsExternal {
		t.Errorf("IsExternal not persisted: got %+v", got)
	}
}

func TestUpsertUserDefaultsIsExternalFalse(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	u := User{ID: "U_INT", WorkspaceID: "T1", Name: "int.user", DisplayName: "Internal"}
	if err := db.UpsertUser(u); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetUser("U_INT")
	if err != nil {
		t.Fatal(err)
	}
	if got.IsExternal {
		t.Errorf("IsExternal should default to false; got %+v", got)
	}
}

func TestUpdatePresence(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	db.UpsertUser(User{ID: "U1", WorkspaceID: "T1", Name: "alice", Presence: "active"})

	if err := db.UpdatePresence("U1", "away"); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetUser("U1")
	if got.Presence != "away" {
		t.Errorf("expected 'away', got %q", got.Presence)
	}
}

// A profile refresh (a message from the user, an avatar fetch) re-upserts
// the row with the "away" placeholder. It must not overwrite the presence
// the WebSocket recorded: the sidebar reseeds DM dots from this column on
// every workspace switch.
func TestUpsertUserPreservesPresence(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	if err := db.UpsertUser(User{ID: "U1", WorkspaceID: "T1", Name: "alice", Presence: "away"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdatePresence("U1", "active"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertUser(User{ID: "U1", WorkspaceID: "T1", Name: "alice", DisplayName: "Alice", Presence: "away"}); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetUser("U1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Presence != "active" {
		t.Errorf("presence = %q; want the live \"active\" kept", got.Presence)
	}
	if got.DisplayName != "Alice" {
		t.Errorf("display_name = %q; profile columns should still update", got.DisplayName)
	}
}

// A full name is kept by every write that carries none, and replaced
// by one that carries another: placeholder upserts and edge records
// without a real_name must not blank it.
func TestUserRealNameSurvivesWritesWithoutOne(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	realName := func() string {
		t.Helper()
		u, err := db.GetUser("U1")
		if err != nil {
			t.Fatal(err)
		}
		return u.RealName
	}

	if err := db.UpsertUser(User{ID: "U1", WorkspaceID: "T1", Name: "ann", DisplayName: "ann", RealName: "Ann Lee"}); err != nil {
		t.Fatal(err)
	}
	if got := realName(); got != "Ann Lee" {
		t.Fatalf("real name after insert = %q; want Ann Lee", got)
	}
	if err := db.UpsertUser(User{ID: "U1", WorkspaceID: "T1", Name: "ann", DisplayName: "ann"}); err != nil {
		t.Fatal(err)
	}
	if got := realName(); got != "Ann Lee" {
		t.Errorf("real name after an upsert without one = %q; want Ann Lee kept", got)
	}
	for _, avatar := range []string{"", "https://cdn/ann.png"} {
		if err := db.UpdateUserFromEdge(EdgeUserUpdate{ID: "U1", Name: "ann", DisplayName: "ann", AvatarURL: avatar}); err != nil {
			t.Fatal(err)
		}
		if got := realName(); got != "Ann Lee" {
			t.Errorf("real name after an edge update without one (avatar %q) = %q; want Ann Lee kept", avatar, got)
		}
		if err := db.UpsertUserFromEdge("T1", EdgeUserUpdate{ID: "U1", Name: "ann", DisplayName: "ann", AvatarURL: avatar}); err != nil {
			t.Fatal(err)
		}
		if got := realName(); got != "Ann Lee" {
			t.Errorf("real name after an edge upsert without one (avatar %q) = %q; want Ann Lee kept", avatar, got)
		}
	}
	if err := db.UpdateUserFromEdge(EdgeUserUpdate{ID: "U1", Name: "ann", DisplayName: "ann", RealName: "Ann Lee-Park"}); err != nil {
		t.Fatal(err)
	}
	if got := realName(); got != "Ann Lee-Park" {
		t.Errorf("real name after an edge update = %q; want Ann Lee-Park", got)
	}
	for i, avatar := range []string{"", "https://cdn/bo.png"} {
		id := []string{"U2", "U3"}[i]
		if err := db.UpsertUserFromEdge("T1", EdgeUserUpdate{ID: id, Name: "bo", DisplayName: "bo", RealName: "Bo Ito", AvatarURL: avatar}); err != nil {
			t.Fatal(err)
		}
		u, err := db.GetUser(id)
		if err != nil {
			t.Fatal(err)
		}
		if u.RealName != "Bo Ito" {
			t.Errorf("real name of an edge-inserted user (avatar %q) = %q; want Bo Ito", avatar, u.RealName)
		}
	}
	users, err := db.ListUsers("T1")
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]string{}
	for _, u := range users {
		listed[u.ID] = u.RealName
	}
	if listed["U1"] != "Ann Lee-Park" || listed["U2"] != "Bo Ito" {
		t.Errorf("ListUsers real names = %v; want U1 Ann Lee-Park and U2 Bo Ito", listed)
	}
}
