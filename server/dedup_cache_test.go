package main

import (
	"testing"
	"time"
)

func TestCheckAndMarkReceivedAtDoesNotSlideWindow(t *testing.T) {
	dc := NewDedupCache(30000, 1000)
	defer dc.Stop()

	if dc.CheckAndMarkReceivedAt(1, 2, 1001) {
		t.Fatal("first call should not be a duplicate")
	}

	dc.mutex.Lock()
	dc.entries["ra:1:2"].Radios[0].SeenAt = time.Now().Add(-900 * time.Millisecond)
	dc.mutex.Unlock()

	if !dc.CheckAndMarkReceivedAt(1, 2, 1001) {
		t.Fatal("second same-source call within 1s should be a duplicate")
	}

	dc.mutex.Lock()
	dc.entries["ra:1:2"].Radios[0].SeenAt = time.Now().Add(-1100 * time.Millisecond)
	dc.mutex.Unlock()

	if dc.CheckAndMarkReceivedAt(1, 2, 1001) {
		t.Fatal("call after the 1s window should not stay blocked by a sliding SeenAt")
	}
}

func TestCheckAndMarkReceivedAtSameSourceWithinWindow(t *testing.T) {
	dc := NewDedupCache(30000, 1200)
	defer dc.Stop()

	if dc.CheckAndMarkReceivedAt(28, 3199, 5090075) {
		t.Fatal("first call should not be a duplicate")
	}
	if !dc.CheckAndMarkReceivedAt(28, 3199, 5090075) {
		t.Fatal("same source within the window should be a duplicate")
	}
}

func TestCheckAndMarkReceivedAtDifferentSourceKept(t *testing.T) {
	dc := NewDedupCache(30000, 1200)
	defer dc.Stop()

	if dc.CheckAndMarkReceivedAt(98, 54645, 5090075) {
		t.Fatal("first call should not be a duplicate")
	}
	if dc.CheckAndMarkReceivedAt(98, 54645, 5090099) {
		t.Fatal("different known source should not be treated as a duplicate on arrival")
	}
	if !dc.CheckAndMarkReceivedAt(98, 54645, 5090099) {
		t.Fatal("copy of the second source should still be suppressed")
	}
}

func TestCheckAndMarkReceivedAtUnknownSourceSoftMatch(t *testing.T) {
	dc := NewDedupCache(30000, 1200)
	defer dc.Stop()

	if dc.CheckAndMarkReceivedAt(1, 2, 0) {
		t.Fatal("first unknown-source call should not be a duplicate")
	}
	if !dc.CheckAndMarkReceivedAt(1, 2, 0) {
		t.Fatal("second unknown-source call within window should soft-match")
	}

	dc2 := NewDedupCache(30000, 1200)
	defer dc2.Stop()
	if dc2.CheckAndMarkReceivedAt(1, 2, 1001) {
		t.Fatal("first known call should not be a duplicate")
	}
	if !dc2.CheckAndMarkReceivedAt(1, 2, 0) {
		t.Fatal("unknown source after known source should soft-match (cannot disprove)")
	}
}

func TestSourcesDisproveReceivedAtDuplicate(t *testing.T) {
	if sourcesDisproveReceivedAtDuplicate(1, 2) != true {
		t.Fatal("different known sources should disprove")
	}
	if sourcesDisproveReceivedAtDuplicate(1, 1) {
		t.Fatal("same sources should not disprove")
	}
	if sourcesDisproveReceivedAtDuplicate(0, 1) {
		t.Fatal("unknown source should not disprove")
	}
	if sourcesDisproveReceivedAtDuplicate(1, 0) {
		t.Fatal("unknown source should not disprove")
	}
}

func TestCallPrimarySource(t *testing.T) {
	call := NewCall()
	call.Units = []CallUnit{{UnitRef: 5090075}}
	if got := callPrimarySource(call); got != 5090075 {
		t.Fatalf("expected 5090075 from Units, got %d", got)
	}
	call2 := NewCall()
	call2.Meta.UnitRefs = []uint{12345}
	if got := callPrimarySource(call2); got != 12345 {
		t.Fatalf("expected 12345 from Meta.UnitRefs, got %d", got)
	}
}

func TestReceivedAtDuplicateWindowFromMs(t *testing.T) {
	if got := receivedAtDuplicateWindowFromMs(0); got != time.Second {
		t.Fatalf("zero should default to 1s, got %v", got)
	}
	if got := receivedAtDuplicateWindowFromMs(1200); got != 1200*time.Millisecond {
		t.Fatalf("1200ms option should pass through, got %v", got)
	}
}
