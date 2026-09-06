// Copyright (C) 2025 Thinline Dynamic Solutions
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>

package main

import (
	"fmt"
	"sync"
	"time"
)

// DedupRadioSeen records when a source (radio ID, or unknown/0) was first seen
// for a system+talkgroup within the arrival match window.
type DedupRadioSeen struct {
	Source uint // upload `source` / unitRef; 0 = unknown
	SeenAt time.Time
}

// DedupEntry caches recent arrivals for a system+talkgroup key so simultaneous
// uploads can be compared before either row is committed.
type DedupEntry struct {
	Radios []DedupRadioSeen
}

// DedupCache is a mutex-protected in-memory cache that closes the race window
// where two copies of the same transmission arrive simultaneously and both pass
// the DB check before either has been written.
//
// Arrival matching is by system+talkgroup and server arrival time, with a soft
// radio-ID guard: when both sides have a known upload `source` and they differ,
// the later call is kept. Missing or zero sources fall back to arrival-time
// matching. Feeder and API key are never considered. The radio-timestamp last
// pass uses the same soft RID rule.
//
// Key prefixes:
//
//	"ra:systemId:talkgroupId" — server arrival-time duplicate entry
//
// A background goroutine evicts stale entries every 30 seconds.
type DedupCache struct {
	entries     map[string]*DedupEntry
	mutex       sync.Mutex
	ttl         time.Duration
	matchWindow time.Duration
	stopCh      chan struct{}
}

func NewDedupCache(timeframeMs, matchWindowMs uint) *DedupCache {
	ttl := time.Duration(timeframeMs*2) * time.Millisecond
	if ttl < 60*time.Second {
		ttl = 60 * time.Second
	}
	dc := &DedupCache{
		entries:     make(map[string]*DedupEntry),
		ttl:         ttl,
		matchWindow: receivedAtDuplicateWindowFromMs(matchWindowMs),
		stopCh:      make(chan struct{}),
	}
	go dc.evictionLoop()
	return dc
}

// CheckAndMarkReceivedAt returns true when a call for the given system+talkgroup
// was already seen within the arrival match window and the soft source guard
// does not disprove the match. Each known source keeps its own SeenAt so a
// second talker's copy can still be suppressed after a different source was kept.
func (dc *DedupCache) CheckAndMarkReceivedAt(systemId, talkgroupId uint64, source uint) bool {
	key := fmt.Sprintf("ra:%d:%d", systemId, talkgroupId)
	now := time.Now()
	dc.mutex.Lock()
	defer dc.mutex.Unlock()

	window := dc.matchWindow
	if window <= 0 {
		window = receivedAtDuplicateWindowFromMs(0)
	}

	entry, ok := dc.entries[key]
	if !ok {
		dc.entries[key] = &DedupEntry{Radios: []DedupRadioSeen{{Source: source, SeenAt: now}}}
		return false
	}

	live := entry.Radios[:0]
	for _, prior := range entry.Radios {
		if now.Sub(prior.SeenAt) <= window {
			live = append(live, prior)
		}
	}
	entry.Radios = live

	for _, prior := range entry.Radios {
		if !sourcesDisproveReceivedAtDuplicate(source, prior.Source) {
			return true
		}
	}

	entry.Radios = append(entry.Radios, DedupRadioSeen{Source: source, SeenAt: now})
	return false
}

func (dc *DedupCache) evictionLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			dc.evict()
		case <-dc.stopCh:
			return
		}
	}
}

func (dc *DedupCache) evict() {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()
	cutoff := time.Now().Add(-dc.ttl)
	for key, entry := range dc.entries {
		live := entry.Radios[:0]
		for _, prior := range entry.Radios {
			if !prior.SeenAt.Before(cutoff) {
				live = append(live, prior)
			}
		}
		if len(live) == 0 {
			delete(dc.entries, key)
			continue
		}
		entry.Radios = live
	}
}

// Stop shuts down the background eviction goroutine.
func (dc *DedupCache) Stop() {
	close(dc.stopCh)
}

// UpdateTTL reconfigures the cache TTL when the timeframe option changes.
func (dc *DedupCache) UpdateTTL(timeframeMs uint) {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()
	ttl := time.Duration(timeframeMs*2) * time.Millisecond
	if ttl < 60*time.Second {
		ttl = 60 * time.Second
	}
	dc.ttl = ttl
}

// UpdateMatchWindow reconfigures the arrival-time match window from options.
func (dc *DedupCache) UpdateMatchWindow(matchWindowMs uint) {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()
	dc.matchWindow = receivedAtDuplicateWindowFromMs(matchWindowMs)
}

// Size returns the current number of entries (for diagnostics).
func (dc *DedupCache) Size() int {
	dc.mutex.Lock()
	defer dc.mutex.Unlock()
	return len(dc.entries)
}
