// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package seccomptracer provides symbolication for stack traces
package seccomptracer

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/procfs"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// procMapsCache caches /proc/pid/maps data to avoid repeated reads
type procMapsCache struct {
	mu         sync.RWMutex
	cache      map[uint32][]*procfs.ProcMap
	timestamps map[uint32]time.Time
	ttl        time.Duration
}

var mapsCache = &procMapsCache{
	cache:      make(map[uint32][]*procfs.ProcMap),
	timestamps: make(map[uint32]time.Time),
	ttl:        5 * time.Second, // Cache for 5 seconds
}

// getProcMaps reads and parses /proc/pid/maps using procfs library
func getProcMaps(pid uint32) ([]*procfs.ProcMap, error) {
	// Check cache first
	mapsCache.mu.RLock()
	if mappings, exists := mapsCache.cache[pid]; exists {
		if time.Since(mapsCache.timestamps[pid]) < mapsCache.ttl {
			mapsCache.mu.RUnlock()
			return mappings, nil
		}
	}
	mapsCache.mu.RUnlock()

	// Use procfs library to read memory maps
	fs, err := procfs.NewFS(kernel.ProcFSRoot())
	if err != nil {
		return nil, fmt.Errorf("failed to create procfs: %w", err)
	}

	proc, err := fs.Proc(int(pid))
	if err != nil {
		return nil, fmt.Errorf("failed to open process %d: %w", pid, err)
	}

	procMaps, err := proc.ProcMaps()
	if err != nil {
		return nil, fmt.Errorf("error reading process memory maps: %w", err)
	}

	// Filter out anonymous mappings and special regions
	var mappings []*procfs.ProcMap
	for _, pm := range procMaps {
		if pm.Pathname != "" && !strings.HasPrefix(pm.Pathname, "[") {
			mappings = append(mappings, pm)
		}
	}

	// Update cache
	mapsCache.mu.Lock()
	mapsCache.cache[pid] = mappings
	mapsCache.timestamps[pid] = time.Now()
	mapsCache.mu.Unlock()

	return mappings, nil
}

// findMapping finds the memory mapping that contains the given address
func findMapping(mappings []*procfs.ProcMap, addr uint64) *procfs.ProcMap {
	for _, mapping := range mappings {
		if addr >= uint64(mapping.StartAddr) && addr < uint64(mapping.EndAddr) {
			return mapping
		}
	}
	return nil
}

// SymbolicateAddresses converts a list of addresses to symbolicated strings
// Returns a slice of strings with function names, line numbers, and inline info when available
func SymbolicateAddresses(pid uint32, addresses []uint64) []string {
	if len(addresses) == 0 {
		return nil
	}

	// Get memory mappings for the process
	mappings, err := getProcMaps(pid)
	if err != nil {
		log.Debugf("failed to get proc maps for pid %d: %v", pid, err)
		// Return raw addresses as fallback
		symbols := make([]string, len(addresses))
		for i, addr := range addresses {
			symbols[i] = fmt.Sprintf("0x%x", addr)
		}
		return symbols
	}

	symbols := make([]string, len(addresses))
	for i, addr := range addresses {
		mapping := findMapping(mappings, addr)
		if mapping != nil {
			// Calculate offset within the binary
			// offset = (address - mapping_start) + file_offset
			offset := (addr - uint64(mapping.StartAddr)) + uint64(mapping.Offset)

			// Create cache key from device and inode
			key := binaryKey{
				dev:   mapping.Dev,
				inode: mapping.Inode,
			}

			// Try to get debug information from cache
			info, err := globalDwarfCache.get(key, mapping.Pathname)
			if err != nil {
				// Failed to load binary info, fall back to simple format
				log.Tracef("Failed to load binary info for %s: %v", mapping.Pathname, err)
				symbols[i] = fmt.Sprintf("%s+0x%x", mapping.Pathname, offset)
			} else {
				// Resolve address using DWARF/symbols
				symbols[i] = resolveAddress(info, offset)
			}
		} else {
			// Couldn't resolve, use raw address
			symbols[i] = fmt.Sprintf("0x%x", addr)
		}
	}

	return symbols
}
