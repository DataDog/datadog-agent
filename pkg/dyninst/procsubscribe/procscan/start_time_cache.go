// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package procscan

// startTimeCache caches process start times keyed by PID.
//
// It stores ticks since boot, and uses the high bit as a "seen in this scan"
// marker.
//
// getStartTime marks any accessed PID as seen. sweep keeps seen entries
// (clearing the marker) and deletes unseen ones (PIDs not observed in procfs).
//
// The cache is bounded by maxSize. When full, new entries are discarded.
//
// The embedded high bit is safe because it would require hundreds of millions
// of years of ticks since boot to be set by a real start time.
type startTimeCache struct {
	entries map[uint32]ticks
	maxSize int
}

// Each entry is 16 bytes, so we'll spend at most roughly 16KiB on the cache
// when full (though the actual memory usage will be higher due to map load
// factors and internal overhead).
const defaultStartTimeCacheSize = 1024

const startTimeCacheSeenBit ticks = ticks(uint64(1) << 63)

func makeStartTimeCache(maxSize int) startTimeCache {
	return startTimeCache{
		entries: make(map[uint32]ticks),
		maxSize: maxSize,
	}
}

func (c *startTimeCache) getStartTime(pid uint32) (ticks, bool) {
	startTime, ok := c.entries[pid]
	if !ok {
		return 0, false
	}
	c.entries[pid] = startTime | startTimeCacheSeenBit
	return startTime &^ startTimeCacheSeenBit, true
}

func (c *startTimeCache) insert(pid uint32, startTime ticks) {
	if c.maxSize > 0 && len(c.entries) >= c.maxSize {
		// Already full.
		return
	}
	c.entries[pid] = startTime | startTimeCacheSeenBit
}

func (c *startTimeCache) sweep() {
	for pid, startTime := range c.entries {
		if startTime&startTimeCacheSeenBit != 0 {
			c.entries[pid] = startTime &^ startTimeCacheSeenBit
			continue
		}
		delete(c.entries, pid)
	}
}
