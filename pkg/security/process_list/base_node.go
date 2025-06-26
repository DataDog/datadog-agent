package processlist

import (
	"time"
)

// VersionTimes holds the first and last seen timestamps for a specific version (image tag).
type VersionTimes struct {
	FirstSeen time.Time
	LastSeen  time.Time
}

type NodeBase struct {
	Seen map[string]*VersionTimes // imageTag → timestamps
}

func NewNodeBase() NodeBase {
	return NodeBase{Seen: make(map[string]*VersionTimes)}
}

// Record updates FirstSeen/LastSeen for the given imageTag at time 'now'.
func (b *NodeBase) Record(imageTag string, now time.Time) {
	if vt, ok := b.Seen[imageTag]; ok {
		if now.Before(vt.FirstSeen) {
			vt.FirstSeen = now
		}
		if now.After(vt.LastSeen) {
			vt.LastSeen = now
		}
		return
	}
	b.Seen[imageTag] = &VersionTimes{FirstSeen: now, LastSeen: now}
}

// EvictVersion removes the stored timestamps for an imageTag.
func (b *NodeBase) EvictVersion(imageTag string) {
	delete(b.Seen, imageTag)
}

// HasVersion returns true if the imageTag exists in the Seen map.
func (b *NodeBase) HasVersion(imageTag string) bool {
	_, exists := b.Seen[imageTag]
	return exists
}