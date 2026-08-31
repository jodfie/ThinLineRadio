package main

import (
	"encoding/json"
	"testing"
)

func TestNormalizeSystemAccessFreezesWildcards(t *testing.T) {
	systems := &Systems{List: []*System{
		{
			SystemRef: 10,
			Talkgroups: &Talkgroups{List: []*Talkgroup{
				{TalkgroupRef: 100},
				{TalkgroupRef: 200},
			}},
		},
		{
			SystemRef: 20,
			Talkgroups: &Talkgroups{List: []*Talkgroup{
				{TalkgroupRef: 300},
			}},
		},
	}}

	t.Run("off freezes All on save", func(t *testing.T) {
		ug := &UserGroup{SystemAccess: "", AutoEnableNewTalkgroups: false}
		ug.NormalizeSystemAccess(systems)
		if ug.SystemAccess == "" {
			t.Fatal("expected frozen snapshot, got empty All")
		}
		if !ug.HasSystemAccess(10) || !ug.HasTalkgroupAccess(10, 100) {
			t.Fatalf("frozen snapshot missing expected access: %s", ug.SystemAccess)
		}
		if ug.HasSystemAccess(99) {
			t.Fatal("frozen All should not grant unknown systems")
		}
	})

	t.Run("startup expand leaves All alone", func(t *testing.T) {
		ug := &UserGroup{SystemAccess: "", AutoEnableNewTalkgroups: false}
		ug.ExpandStarTalkgroups(systems)
		if ug.SystemAccess != "" {
			t.Fatalf("startup must not rewrite All, got %q", ug.SystemAccess)
		}
	})

	t.Run("off expands star talkgroups", func(t *testing.T) {
		ug := &UserGroup{
			SystemAccess:            `[{"id":10,"talkgroups":"*"}]`,
			AutoEnableNewTalkgroups: false,
		}
		ug.ExpandStarTalkgroups(systems)
		ug.loadSystemAccess()
		if ug.HasTalkgroupAccess(10, 100) != true || ug.HasTalkgroupAccess(10, 200) != true {
			t.Fatalf("expected current TGs granted: %s", ug.SystemAccess)
		}
		// Simulate a talkgroup that did not exist at freeze time.
		systems.List[0].Talkgroups.List = append(systems.List[0].Talkgroups.List, &Talkgroup{TalkgroupRef: 999})
		if ug.HasTalkgroupAccess(10, 999) {
			t.Fatal("new talkgroup must not be granted after freeze")
		}
	})

	t.Run("on keeps wildcards", func(t *testing.T) {
		ug := &UserGroup{SystemAccess: "", AutoEnableNewTalkgroups: true}
		ug.NormalizeSystemAccess(systems)
		if ug.SystemAccess != "" {
			t.Fatalf("expected All wildcard preserved, got %q", ug.SystemAccess)
		}
		ug.loadSystemAccess()
		if !ug.HasSystemAccess(99) {
			t.Fatal("All with auto-enable should still grant any system")
		}
	})
}

func TestExplicitEmptySystemAccessDeniesAll(t *testing.T) {
	ug := &UserGroup{SystemAccess: "[]"}
	ug.loadSystemAccess()
	if ug.HasAnySystemAccess() {
		t.Fatal("explicit [] must deny all systems")
	}
	if ug.HasSystemAccess(1) {
		t.Fatal("explicit [] must deny system access")
	}
}

func TestNormalizeSystemAccessRoundTripJSON(t *testing.T) {
	systems := &Systems{List: []*System{
		{SystemRef: 1, Talkgroups: &Talkgroups{List: []*Talkgroup{{TalkgroupRef: 5}}}},
	}}
	ug := &UserGroup{SystemAccess: "", AutoEnableNewTalkgroups: false}
	ug.NormalizeSystemAccess(systems)
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(ug.SystemAccess), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 system, got %d", len(parsed))
	}
}
