package main

import (
	"testing"
	"time"
)

func TestCrossTalkgroupEmitKeyRequiresSource(t *testing.T) {
	sys := &System{SystemRef: 78}
	call := NewCall()
	call.System = sys
	call.Timestamp = time.UnixMilli(1788637901000)
	if _, ok := crossTalkgroupEmitKey(call); ok {
		t.Fatal("zero source should not produce a key")
	}
	call.Units = []CallUnit{{UnitRef: 7890436}}
	key, ok := crossTalkgroupEmitKey(call)
	if !ok || key == "" {
		t.Fatal("expected key with source")
	}
	call2 := NewCall()
	call2.System = sys
	call2.Timestamp = call.Timestamp
	call2.Units = []CallUnit{{UnitRef: 7890436}}
	call2.Talkgroup = &Talkgroup{TalkgroupRef: 46024}
	key2, _ := crossTalkgroupEmitKey(call2)
	if key != key2 {
		t.Fatalf("same system/source/ts should share key across talkgroups: %q vs %q", key, key2)
	}
}

func TestEmitDedupeSkipsSecondTalkgroup(t *testing.T) {
	d := &EmitDedupe{}
	sys := &System{SystemRef: 78, Talkgroups: NewTalkgroups()}
	cent := &Talkgroup{TalkgroupRef: 46028, Label: "CENT"}
	se := &Talkgroup{TalkgroupRef: 46024, Label: "SE"}
	sys.Talkgroups.List = []*Talkgroup{cent, se}

	a := NewCall()
	a.System = sys
	a.Talkgroup = cent
	a.Timestamp = time.UnixMilli(1788637901000)
	a.Units = []CallUnit{{UnitRef: 7890436}}

	b := NewCall()
	b.System = sys
	b.Talkgroup = se
	b.Timestamp = a.Timestamp
	b.Units = []CallUnit{{UnitRef: 7890436}}

	if d.ShouldSkip(a) {
		t.Fatal("first call should emit")
	}
	if !d.ShouldSkip(b) {
		t.Fatal("second talkgroup copy should be skipped")
	}

	// Different radio timestamp = different PTT
	c := NewCall()
	c.System = sys
	c.Talkgroup = se
	c.Timestamp = time.UnixMilli(1788637901000 + 5000)
	c.Units = []CallUnit{{UnitRef: 7890436}}
	if d.ShouldSkip(c) {
		t.Fatal("different radio timestamp should not skip")
	}
}

func TestEmitDedupeSoftWithoutSource(t *testing.T) {
	d := &EmitDedupe{}
	sys := &System{SystemRef: 1}
	a := NewCall()
	a.System = sys
	a.Talkgroup = &Talkgroup{TalkgroupRef: 1}
	a.Timestamp = time.UnixMilli(1000)
	b := NewCall()
	b.System = sys
	b.Talkgroup = &Talkgroup{TalkgroupRef: 2}
	b.Timestamp = a.Timestamp
	if d.ShouldSkip(a) || d.ShouldSkip(b) {
		t.Fatal("unknown source must not cross-talkgroup dedupe")
	}
}
