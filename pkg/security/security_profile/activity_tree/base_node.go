package activitytree

import (
	"time"
)

// ImageTagTimes holds the first and last seen timestamps for a specific ImageTag (image tag).
type ImageTagTimes struct {
	FirstSeen time.Time
	LastSeen  time.Time
}

type NodeBase struct {
	Seen map[string]*ImageTagTimes // imageTag → timestamps
}

func NewNodeBase() NodeBase {
	return NodeBase{Seen: make(map[string]*ImageTagTimes)}
}

// Add new entry in the map or update the LastSeen for the given imageTag at time 'now'.
func (b *NodeBase) AppendImageTag (imageTag string, timestamp time.Time) {
	if imageTag == "" {
		return
	}
	if vt, ok := b.Seen[imageTag]; ok {
		vt.LastSeen = timestamp
		return
	}
	b.Seen[imageTag] = &ImageTagTimes{FirstSeen: timestamp, LastSeen: timestamp}
}

// RecordWithTimestamps sets both FirstSeen and LastSeen for the given imageTag with the provided timestamps.
func (b *NodeBase) RecordWithTimestamps(imageTag string, firstSeen, lastSeen time.Time) {
	b.Seen[imageTag] = &ImageTagTimes{FirstSeen: firstSeen, LastSeen: lastSeen}
}

// EvictImageTag removes the stored timestamps for an imageTag returns true if the imageTag was present
func (b *NodeBase) EvictImageTag(imageTag string) bool {
	_, exists := b.Seen[imageTag]
	delete(b.Seen, imageTag)
	return exists
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

// HasImageTag returns true if the imageTag exists in the Seen map.
func (b *NodeBase) HasImageTag(imageTag string) bool {
	_, exists := b.Seen[imageTag]
	return exists
}

//add a function that checks if the seen map is empty
func (b *NodeBase) IsEmpty() bool {
	return len(b.Seen) == 0
}
