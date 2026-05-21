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

// ImageTagTimes holds the first and last seen timestamps for a specific ImageTag (image tag).
type ImageTagTimes struct {
	FirstSeen time.Time
	LastSeen  time.Time
}

// NodeBase provides the base functionality for all nodes in the activity tree
type NodeBase struct {
	Seen map[uint64]ImageTagTimes // imageTag → timestamps
}

// NewNodeBase creates a new NodeBase instance
func NewNodeBase() NodeBase {
	return NodeBase{Seen: make(map[uint64]ImageTagTimes)}
}

// AppendImageTagID adds a new entry in the map or updates the LastSeen for the given imageTag at time 'now'.
func (b *NodeBase) AppendImageTagID(imageTagID uint64, timestamp time.Time) {
	if imageTagID == 0 {
		return
	}
	if vt, ok := b.Seen[imageTagID]; ok {
		vt.LastSeen = timestamp
		b.Seen[imageTagID] = vt
		return
	}
	b.Seen[imageTagID] = ImageTagTimes{FirstSeen: timestamp, LastSeen: timestamp}
}

// RecordWithTimestamps sets both FirstSeen and LastSeen for the given imageTagID with the provided timestamps.
func (b *NodeBase) RecordWithTimestamps(imageTagID uint64, firstSeen, lastSeen time.Time) {
	b.Seen[imageTagID] = ImageTagTimes{FirstSeen: firstSeen, LastSeen: lastSeen}
}

// EvictImageTag removes the stored timestamps for an imageTagID returns false if the imageTag was not present or if the imageTag is empty
// returns true if the imageTag was present and the map is now empty
func (b *NodeBase) EvictImageTag(imageTagID uint64) bool {
	if !b.HasImageTag(imageTagID) {
		return false
	}
	delete(b.Seen, imageTagID)
	return len(b.Seen) == 0
}

// EvictBeforeTimestamp removes all imageTags whose LastSeen is before the given timestamp.
// Returns the number of imageTags that were removed.
func (b *NodeBase) EvictBeforeTimestamp(before time.Time) int {
	removed := 0
	for imageTag, times := range b.Seen {
		if times.LastSeen.Before(before) {
			delete(b.Seen, imageTag)
			removed++
		}
	}
	return removed
}

// HasImageTag returns true if the imageTagID exists in the Seen map.
func (b *NodeBase) HasImageTag(imageTagID uint64) bool {
	_, exists := b.Seen[imageTagID]
	return exists
}
