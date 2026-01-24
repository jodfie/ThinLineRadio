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

// QueuedCall represents a call waiting in the queue for preferred site resolution
type QueuedCall struct {
	Call      *Call
	Timer     *time.Timer
	ExpiresAt time.Time
}

// CallQueue manages pending secondary site calls waiting for preferred site resolution
type CallQueue struct {
	queue map[string]*QueuedCall // key: "systemId-talkgroupId-timestamp"
	mutex sync.RWMutex
}

// NewCallQueue creates a new call queue
func NewCallQueue() *CallQueue {
	return &CallQueue{
		queue: make(map[string]*QueuedCall),
		mutex: sync.RWMutex{},
	}
}

// generateKey creates a unique key for queued calls
func (cq *CallQueue) generateKey(call *Call) string {
	return fmt.Sprintf("%d-%d-%d", call.System.Id, call.Talkgroup.Id, call.Timestamp.UnixMilli())
}

// Add adds a call to the queue with a timer
func (cq *CallQueue) Add(call *Call, waitDuration time.Duration, onExpire func(*Call)) {
	cq.mutex.Lock()
	defer cq.mutex.Unlock()

	key := cq.generateKey(call)

	// Create timer that will process the call after waiting period
	timer := time.AfterFunc(waitDuration, func() {
		cq.mutex.Lock()
		defer cq.mutex.Unlock()

		// Check if call still exists in queue (wasn't cancelled by preferred site)
		if queuedCall, exists := cq.queue[key]; exists {
			delete(cq.queue, key)
			onExpire(queuedCall.Call)
		}
	})

	cq.queue[key] = &QueuedCall{
		Call:      call,
		Timer:     timer,
		ExpiresAt: time.Now().Add(waitDuration),
	}
}

// CancelPending cancels all pending secondary site calls for the given system/talkgroup within time window
func (cq *CallQueue) CancelPending(systemId uint64, talkgroupId uint64, timestamp time.Time, timeWindow time.Duration) int {
	cq.mutex.Lock()
	defer cq.mutex.Unlock()

	cancelled := 0
	from := timestamp.Add(-timeWindow)
	to := timestamp.Add(timeWindow)

	// Find and cancel all matching queued calls
	for key, queuedCall := range cq.queue {
		if queuedCall.Call.System.Id == systemId &&
			queuedCall.Call.Talkgroup.Id == talkgroupId &&
			queuedCall.Call.Timestamp.After(from) &&
			queuedCall.Call.Timestamp.Before(to) {

			// Stop the timer
			queuedCall.Timer.Stop()

			// Remove from queue
			delete(cq.queue, key)
			cancelled++
		}
	}

	return cancelled
}

// GetQueueSize returns the current number of queued calls
func (cq *CallQueue) GetQueueSize() int {
	cq.mutex.RLock()
	defer cq.mutex.RUnlock()
	return len(cq.queue)
}

// Cleanup removes expired entries (defensive cleanup)
func (cq *CallQueue) Cleanup() {
	cq.mutex.Lock()
	defer cq.mutex.Unlock()

	now := time.Now()
	for key, queuedCall := range cq.queue {
		if now.After(queuedCall.ExpiresAt) {
			queuedCall.Timer.Stop()
			delete(cq.queue, key)
		}
	}
}
