// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadCUOccupancy returns, per device UUID, the compute units occupied by the
// waves in flight of all processes at the time of the call, from
// /sys/class/kfd/kfd/proc/<pid>/stats_<gpu_id>/cu_occupancy. Partitions are
// summed onto the physical GPU.
//
// The kernel computes each value on read (kfd_get_cu_occupancy): it reads the
// in-flight wave count of every compute queue from SPI registers, keeps the
// queues of the process, and rounds the waves up to whole compute units. It is
// a snapshot, not an interval average. On multi-XCC GPUs (GC 9.4.3 and later)
// only the first XCC of a partition is read and the count is multiplied by the
// number of XCCs; this is only correct from Linux 6.12. On SR-IOV virtual
// functions the registers read as 0.
//
// complete has the meaning of ReadProcessMemory: no sums are returned while a
// KFD node cannot be mapped to a physical GPU.
func ReadCUOccupancy(sysRoot string, devices []*Device) (map[string]uint64, bool, error) {
	gpuIDToUUID := make(map[uint64]string)
	for _, dev := range devices {
		if dev.KFDAccessDenied || dev.kfdTopologyIncomplete {
			return nil, false, nil
		}
		for _, id := range dev.kfdGPUIDs {
			gpuIDToUUID[id] = dev.UUID
		}
	}
	if len(gpuIDToUUID) == 0 {
		return nil, true, nil
	}

	occupied := make(map[string]uint64)
	procDir := filepath.Join(sysRoot, "class", "kfd", "kfd", "proc")
	entries, err := os.ReadDir(procDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return occupied, true, nil
		}
		return nil, false, fmt.Errorf("list %s: %w", procDir, err)
	}

	var errs []error
	for _, entry := range entries {
		pid, err := strconv.ParseUint(entry.Name(), 10, 31)
		if err != nil || pid == 0 || !entry.IsDir() {
			continue
		}
		if err := readKFDProcessOccupancy(filepath.Join(procDir, entry.Name()), gpuIDToUUID, occupied, true); err != nil {
			errs = append(errs, err)
		}
	}
	err = errors.Join(errs...)
	return occupied, err == nil, err
}

func readKFDProcessOccupancy(dir string, gpuIDToUUID map[uint64]string, occupied map[string]uint64, includeContexts bool) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // process or context exited
		}
		return err
	}
	var errs []error
	for _, file := range files {
		if !file.IsDir() {
			continue
		}
		if id, context := strings.CutPrefix(file.Name(), "context_"); context {
			if _, err := strconv.ParseUint(id, 10, 32); err == nil && includeContexts {
				if err := readKFDProcessOccupancy(filepath.Join(dir, file.Name()), gpuIDToUUID, occupied, false); err != nil {
					errs = append(errs, err)
				}
			}
			continue
		}
		id, stats := strings.CutPrefix(file.Name(), "stats_")
		if !stats {
			continue
		}
		gpuID, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			continue
		}
		uuid, known := gpuIDToUUID[gpuID]
		if !known {
			continue
		}
		// Absent when the ASIC has no occupancy callback (get_cu_occupancy).
		cus, err := readUint(filepath.Join(dir, file.Name(), "cu_occupancy"))
		if err != nil {
			if !isUnsupported(err) {
				errs = append(errs, err)
			}
			continue
		}
		occupied[uuid] += cus
	}
	return errors.Join(errs...)
}
