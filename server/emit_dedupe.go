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

// crossTalkgroupEmitDedupeWindow is how long a system+source+radio-timestamp
// fingerprint blocks additional livefeed emits to the same client. Covers
// multi-talkgroup copies of one PTT (dispatcher multi-select, site fans, etc.)
// without being tied to any specific system or talkgroup set.
const crossTalkgroupEmitDedupeWindow = 5 * time.Second

// EmitDedupe tracks recent livefeed emits per client (or reconnect buffer) so
// the same transmission is only streamed once when it lands as separate calls
// on different talkgroups.
type EmitDedupe struct {
	mu     sync.Mutex
	recent map[string]time.Time
}

// crossTalkgroupEmitKey returns a fingerprint for universal cross-talkgroup
// emit dedupe. Soft: missing/zero source or radio timestamp → ok=false (do not
// dedupe; typical VHF analog / unknown unit).
func crossTalkgroupEmitKey(call *Call) (key string, ok bool) {
	if call == nil || call.System == nil {
		return "", false
	}
	src := callPrimarySource(call)
	if src == 0 {
		return "", false
	}
	ts := call.Timestamp.UnixMilli()
	if ts == 0 {
		return "", false
	}
	return fmt.Sprintf("%d:%d:%d", call.System.SystemRef, src, ts), true
}

// ShouldSkip reports whether this call should not be streamed to the client
// because an equivalent transmission (same system, source, radio timestamp)
// was already accepted within the window. First sighting marks the key.
func (d *EmitDedupe) ShouldSkip(call *Call) bool {
	if d == nil {
		return false
	}
	key, ok := crossTalkgroupEmitKey(call)
	if !ok {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.recent == nil {
		d.recent = make(map[string]time.Time)
	}

	now := time.Now()
	for k, seen := range d.recent {
		if now.Sub(seen) > crossTalkgroupEmitDedupeWindow {
			delete(d.recent, k)
		}
	}

	if seen, exists := d.recent[key]; exists && now.Sub(seen) <= crossTalkgroupEmitDedupeWindow {
		return true
	}
	d.recent[key] = now
	return false
}
