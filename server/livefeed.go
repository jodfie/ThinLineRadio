// Copyright (C) 2019-2024 Chrystian Huot <chrystian@huot.qc.ca>
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
	"strconv"
	"sync"
)

type Livefeed struct {
	Matrix map[uint]map[uint]bool
	mutex  sync.Mutex
}

func NewLivefeed() *Livefeed {
	return &Livefeed{
		Matrix: map[uint]map[uint]bool{},
		mutex:  sync.Mutex{},
	}
}

func (livefeed *Livefeed) FromMap(f any) *Livefeed {
	livefeed.mutex.Lock()
	defer livefeed.mutex.Unlock()

	for s := range livefeed.Matrix {
		delete(livefeed.Matrix, s)
	}

	switch v := f.(type) {
	case map[string]any:
		for s, n := range v {
			if sysId, err := strconv.Atoi(s); err == nil {
				sysId := uint(sysId)
				switch v := n.(type) {
				case map[string]any:
					for t, b := range v {
						switch v := b.(type) {
						case bool:
							if tgId, err := strconv.Atoi(t); err == nil {
								tgId := uint(tgId)
								if livefeed.Matrix[sysId] == nil {
									livefeed.Matrix[sysId] = map[uint]bool{}
								}
								livefeed.Matrix[sysId][tgId] = v
							}
						}
					}
				}
			}
		}
	}

	return livefeed
}

func (livefeed *Livefeed) IsAllOff() bool {
	livefeed.mutex.Lock()
	defer livefeed.mutex.Unlock()

	for _, sys := range livefeed.Matrix {
		for _, tg := range sys {
			if tg {
				return false
			}
		}
	}

	return true
}

// IsEnabledForRef returns true if the client has the given systemRef+talkgroupRef
// pair active in their livefeed. Used for cross-talkgroup duplicate filtering.
func (livefeed *Livefeed) IsEnabledForRef(systemRef, talkgroupRef uint) bool {
	livefeed.mutex.Lock()
	defer livefeed.mutex.Unlock()
	return livefeed.Matrix[systemRef][talkgroupRef]
}

func (livefeed *Livefeed) IsEnabled(call *Call) bool {
	return len(livefeed.EnabledMatchingTalkgroupRefs(call)) > 0
}

// EnabledMatchingTalkgroupRefs returns the client's enabled talkgroup refs that
// match this call — primary first (if on), then any patch members that are on.
// Used so a patched call is streamed once as a single talkgroup, not via PATCH
// fan-in when multiple members are selected.
func (livefeed *Livefeed) EnabledMatchingTalkgroupRefs(call *Call) []uint {
	if livefeed == nil || call == nil || call.System == nil || call.Talkgroup == nil {
		return nil
	}

	livefeed.mutex.Lock()
	defer livefeed.mutex.Unlock()

	sys := call.System.SystemRef
	primary := call.Talkgroup.TalkgroupRef
	out := make([]uint, 0, 1+len(call.Patches))

	if livefeed.Matrix[sys][primary] {
		out = append(out, primary)
	}
	for _, p := range call.Patches {
		if p == 0 || p == primary {
			continue
		}
		if livefeed.Matrix[sys][p] {
			out = append(out, p)
		}
	}
	return out
}

// ScrubToScopedSystems turns off any livefeed entries that are outside the
// client's currently scoped systems map (group/user ACL). Prevents hearing
// traffic for talkgroups that are no longer (or never were) in the plan.
func (livefeed *Livefeed) ScrubToScopedSystems(systemsMap SystemsMap) {
	livefeed.mutex.Lock()
	defer livefeed.mutex.Unlock()

	allowed := make(map[uint]map[uint]struct{}, len(systemsMap))
	for _, sys := range systemsMap {
		var sysId uint
		switch v := sys["id"].(type) {
		case uint:
			sysId = v
		case uint64:
			sysId = uint(v)
		case int:
			sysId = uint(v)
		case float64:
			sysId = uint(v)
		default:
			continue
		}
		tgSet := make(map[uint]struct{})
		switch tgs := sys["talkgroups"].(type) {
		case TalkgroupsMap:
			for _, tg := range tgs {
				switch id := tg["id"].(type) {
				case uint:
					tgSet[id] = struct{}{}
				case uint64:
					tgSet[uint(id)] = struct{}{}
				case int:
					tgSet[uint(id)] = struct{}{}
				case float64:
					tgSet[uint(id)] = struct{}{}
				}
			}
		}
		allowed[sysId] = tgSet
	}

	for sysId, tgs := range livefeed.Matrix {
		allowedTgs, sysOk := allowed[sysId]
		for tgId, on := range tgs {
			if !on {
				continue
			}
			if !sysOk {
				livefeed.Matrix[sysId][tgId] = false
				continue
			}
			if _, ok := allowedTgs[tgId]; !ok {
				livefeed.Matrix[sysId][tgId] = false
			}
		}
	}
}
