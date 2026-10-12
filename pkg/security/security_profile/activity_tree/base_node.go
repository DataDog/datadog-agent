// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"time"
)

type seenEntry struct {
	id        uint64
	firstSeen int64
	lastSeen  int64
}

func timeToNanos(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// NodeBase provides the base functionality for all nodes in the activity tree.
type NodeBase struct {
	seen []seenEntry
}

// NewNodeBase creates a new NodeBase instance
func NewNodeBase() NodeBase {
	return NodeBase{}
}

// AppendImageTagID adds a new entry in the slice or updates the LastSeen for the given imageTagID.
// ID 0 is the null sentinel and is a no-op.
func (b *NodeBase) AppendImageTagID(imageTagID uint64, timestamp time.Time) {
	if imageTagID == 0 {
		return
	}
	ns := timeToNanos(timestamp)
	for i, entry := range b.seen {
		if entry.id == imageTagID {
			b.seen[i].lastSeen = ns
			return
		}
	}
	// Three-index slice sets cap == len before append so Go allocates exactly one new slot,
	// avoiding the default doubling strategy for a structure that stays small (typically ≤5 entries).
	b.seen = append(b.seen[:len(b.seen):len(b.seen)], seenEntry{id: imageTagID, firstSeen: ns, lastSeen: ns})
}

// RecordWithTimestamps sets both FirstSeen and LastSeen for the given imageTagID with the provided timestamps.
// ID 0 is the null sentinel and is a no-op.
func (b *NodeBase) RecordWithTimestamps(imageTagID uint64, firstSeen, lastSeen time.Time) {
	if imageTagID == 0 {
		return
	}
	fs, ls := timeToNanos(firstSeen), timeToNanos(lastSeen)
	for i, entry := range b.seen {
		if entry.id == imageTagID {
			b.seen[i].firstSeen = fs
			b.seen[i].lastSeen = ls
			return
		}
	}
	b.seen = append(b.seen[:len(b.seen):len(b.seen)], seenEntry{id: imageTagID, firstSeen: fs, lastSeen: ls})
}

// EvictImageTag removes the entry for imageTagID and returns true if the slice is now empty.
// Returns false if imageTagID was not present.
func (b *NodeBase) EvictImageTag(imageTagID uint64) bool {
	for i, entry := range b.seen {
		if entry.id == imageTagID {
			// swap-and-truncate — order doesn't matter for this structure
			b.seen[i] = b.seen[len(b.seen)-1]
			b.seen = b.seen[:len(b.seen)-1]
			return len(b.seen) == 0
		}
	}
	return false
}

// EvictBeforeTimestamp removes all entries whose LastSeen is before the given timestamp.
// Returns the number of entries removed.
func (b *NodeBase) EvictBeforeTimestamp(before time.Time) int {
	beforeNs := timeToNanos(before)
	removed := 0
	i := 0
	for i < len(b.seen) {
		if b.seen[i].lastSeen < beforeNs {
			b.seen[i] = b.seen[len(b.seen)-1]
			b.seen = b.seen[:len(b.seen)-1]
			removed++
		} else {
			i++
		}
	}
	return removed
}

// HasImageTag returns true if imageTagID has an entry in the slice.
func (b *NodeBase) HasImageTag(imageTagID uint64) bool {
	for _, entry := range b.seen {
		if entry.id == imageTagID {
			return true
		}
	}
	return false
}

// SeenIsEmpty returns true if no image tags are recorded.
func (b *NodeBase) SeenIsEmpty() bool {
	return len(b.seen) == 0
}

// SeenLen returns the number of recorded image tags.
func (b *NodeBase) SeenLen() int {
	return len(b.seen)
}

// GetSeenTimes returns the first and last seen timestamps (unix nanoseconds, 0 = unset) for the
// given imageTagID, or (0, 0, false) if not found.
func (b *NodeBase) GetSeenTimes(imageTagID uint64) (firstSeen, lastSeen int64, found bool) {
	for _, entry := range b.seen {
		if entry.id == imageTagID {
			return entry.firstSeen, entry.lastSeen, true
		}
	}
	return 0, 0, false
}

// EachSeen calls fn for every recorded image tag ID and its first/last seen timestamps
// (unix nanoseconds, 0 = unset).
func (b *NodeBase) EachSeen(fn func(id uint64, firstSeen, lastSeen int64)) {
	for _, entry := range b.seen {
		fn(entry.id, entry.firstSeen, entry.lastSeen)
	}
}
