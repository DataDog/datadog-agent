// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package maps

import (
	"fmt"
	"os"
	"strconv"
	"unsafe"

	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// extractPID extracts the PID from a uint64 key where PID is in the upper 32 bits
func extractPID(pidTGID uint64) uint32 {
	return uint32(pidTGID >> 32)
}

// pidExists checks if a process with the given PID exists.
// It uses kernel.HostProc() to correctly handle containerized environments
// where the agent runs in a container but needs to check PIDs from the host namespace.
func pidExists(pid uint32) bool {
	_, err := os.Stat(kernel.HostProc(strconv.FormatUint(uint64(pid), 10)))
	return err == nil
}

// ValidatePIDKeyedMap checks a map with uint64 (pid_tgid) keys for leaked entries
// Returns MapLeakInfo with details about leaked entries
func ValidatePIDKeyedMap(mapName string, m *ebpf.Map) (*MapLeakInfo, error) {
	if m == nil {
		return nil, fmt.Errorf("map %s is nil", mapName)
	}

	info := &MapLeakInfo{
		MapName:  mapName,
		DeadPIDs: make([]uint32, 0),
	}

	// Iterate through all map entries
	iter := m.Iterate()

	// We don't need the value, just the key to check PIDs
	// Use a dummy value buffer that matches the map's value size
	mapInfo, err := m.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get map info for %s: %w", mapName, err)
	}

	var key uint64
	// Validate that the map has uint64 keys as expected
	if mapInfo.KeySize != uint32(unsafe.Sizeof(key)) {
		return nil, fmt.Errorf("map %s has unexpected key size %d, expected %d bytes for uint64", mapName, mapInfo.KeySize, unsafe.Sizeof(key))
	}

	valueSize := mapInfo.ValueSize
	value := make([]byte, valueSize)

	seenPIDs := make(map[uint32]bool)
	for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value[0])) {
		info.TotalEntries++

		pid := extractPID(key)

		// Check if we've already validated this PID
		if alive, seen := seenPIDs[pid]; seen {
			if !alive {
				info.LeakedEntries++
			}
			continue
		}

		// Validate PID exists
		exists := pidExists(pid)
		seenPIDs[pid] = exists

		if !exists {
			info.LeakedEntries++
			// Only add to DeadPIDs list if we haven't seen it yet
			info.DeadPIDs = append(info.DeadPIDs, pid)
		}
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("error iterating map %s: %w", mapName, err)
	}

	// Calculate leak rate
	if info.TotalEntries > 0 {
		info.LeakRate = float64(info.LeakedEntries) / float64(info.TotalEntries)
	}

	return info, nil
}
