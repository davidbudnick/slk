package core

import "testing"

// NewSixelAvatarService hands out sixel avatars through the optional
// SixelAvatarService; without a sixel source it is a plain
// AvatarService, so the App keeps avatars half-block.
func TestNewSixelAvatarService(t *testing.T) {
	half := func(string) string { return "HB" }
	s := NewSixelAvatarService(half, nil, func(userID string) (string, []byte, bool) {
		return "avatar-" + userID, []byte{1, 2}, true
	})
	sx, ok := s.(SixelAvatarService)
	if !ok {
		t.Fatal("service with a sixel source is not a SixelAvatarService")
	}
	if key, b, ok := sx.SixelAvatar("U1"); !ok || key != "avatar-U1" || len(b) != 2 {
		t.Errorf("SixelAvatar(U1) = (%q, %v, %v)", key, b, ok)
	}
	if got := s.Avatar("U1"); got != "HB" {
		t.Errorf("Avatar(U1) = %q, want the half-block avatar", got)
	}
	if _, ok := NewSixelAvatarService(half, nil, nil).(SixelAvatarService); ok {
		t.Error("service without a sixel source claims to be a SixelAvatarService")
	}
}

func TestNewSidebarAvatarService(t *testing.T) {
	plain := NewAvatarService(func(string) string { return "HB" }, nil)
	s := NewSidebarAvatarService(plain, func(userID string, badge AvatarBadge) string {
		if badge.Hollow {
			return "away-" + userID
		}
		return "on-" + userID
	}, func(c CountImage) string {
		return "count-" + c.Label
	})
	sb, ok := s.(SidebarAvatarService)
	if !ok {
		t.Fatal("service with a sidebar source is not a SidebarAvatarService")
	}
	if got := sb.SidebarAvatar("U1", AvatarBadge{Hollow: true}); got != "away-U1" {
		t.Errorf("SidebarAvatar(U1) = %q, want the badged avatar", got)
	}
	if got := sb.SidebarCount(CountImage{Label: "3"}); got != "count-3" {
		t.Errorf("SidebarCount(3) = %q, want the count's image", got)
	}
	if got := s.Avatar("U1"); got != "HB" {
		t.Errorf("Avatar(U1) = %q, want the wrapped service's avatar", got)
	}
	if _, ok := NewSidebarAvatarService(plain, nil, nil).(SidebarAvatarService); ok {
		t.Error("service without a sidebar source claims to be a SidebarAvatarService")
	}
	noCount := NewSidebarAvatarService(plain, func(string, AvatarBadge) string { return "" }, nil).(SidebarAvatarService)
	if got := noCount.SidebarCount(CountImage{Label: "3"}); got != "" {
		t.Errorf("SidebarCount with no count source = %q, want empty", got)
	}
}
