package processlist

import (
	"sync"
)

// VersionTimes holds the first and last seen timestamps for a specific version (image tag).
type VersionTimes struct {
	FirstSeen uint64
	LastSeen  uint64
}

type NodeBase struct {
	sync.Mutex
	Seen map[string]*VersionTimes // version → timestamps
}

func NewNodeBase() NodeBase {
	return NodeBase{Seen: make(map[string]*VersionTimes)}
}

// Record updates FirstSeen/LastSeen for the given version at time 'now'.
func (b *NodeBase) Record(version string, now uint64) {
	b.Lock()
	defer b.Unlock()
	if vt, ok := b.Seen[version]; ok {
		if now < vt.FirstSeen {
			vt.FirstSeen = now
		}
		if now > vt.LastSeen {
			vt.LastSeen = now
		}
		return
	}
	b.Seen[version] = &VersionTimes{FirstSeen: now, LastSeen: now}
}

// EvictVersion removes the stored timestamps for a version.
func (b *NodeBase) EvictVersion(version string) {
	b.Lock()
	defer b.Unlock()
	delete(b.Seen, version)
}