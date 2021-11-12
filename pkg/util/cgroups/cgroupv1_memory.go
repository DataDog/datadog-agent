// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package cgroups

import (
	"errors"
	"math"
	"os"
)

// When no memory limit is set, the Kernel returns a maximum value, being computed as:
// See https://unix.stackexchange.com/questions/420906/what-is-the-value-for-the-cgroups-limit-in-bytes-if-the-memory-is-not-restricte
var memoryUnlimitedValue = (uint64(math.MaxInt64) / uint64(os.Getpagesize())) * uint64(os.Getpagesize())

func (c *cgroupV1) GetMemoryStats(stats *MemoryStats) error {
	if stats == nil {
		return &InvalidInputError{Desc: "input stats cannot be nil"}
	}

	if !c.controllerMounted("memory") {
		return &ControllerNotFoundError{Controller: "memory"}
	}

	if err := parse2ColumnStatsWithMapping(c.fr, c.pathFor("memory", "memory.stat"), 0, 1, map[string]**uint64{
		"total_cache":         &stats.Cache,
		"total_swap":          &stats.Swap,
		"total_rss":           &stats.RSS,
		"total_rss_huge":      &stats.RSSHuge,
		"total_mapped_file":   &stats.MappedFile,
		"total_pgpgin":        &stats.Pgpgin,
		"total_pgpgout":       &stats.Pgpgout,
		"total_pgfault":       &stats.Pgfault,
		"total_pgmajfault":    &stats.Pgmajfault,
		"total_inactive_anon": &stats.InactiveAnon,
		"total_active_anon":   &stats.ActiveAnon,
		"total_inactive_file": &stats.InactiveFile,
		"total_active_file":   &stats.ActiveFile,
		"total_unevictable":   &stats.Unevictable,
	}); err != nil {
		reportError(err)
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory", "memory.usage_in_bytes"), &stats.UsageTotal); err != nil {
		reportError(err)
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory", "memory.failcnt"), &stats.OOMEvents); err != nil {
		reportError(err)
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory", "memory.kmem.usage_in_bytes"), &stats.KernelMemory); err != nil {
		reportError(err)
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory", "memory.limit_in_bytes"), &stats.Limit); err != nil {
		reportError(err)
	}
	if stats.Limit != nil && *stats.Limit >= memoryUnlimitedValue {
		stats.Limit = nil
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory", "memory.soft_limit_in_bytes"), &stats.LowThreshold); err != nil {
		reportError(err)
	}
	if stats.LowThreshold != nil && *stats.LowThreshold >= memoryUnlimitedValue {
		stats.LowThreshold = nil
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory", "memory.memsw.limit_in_bytes"), &stats.SwapLimit); err != nil {
		// Not adding error for `memsw` as the file is not always present (requires swap to be enabled)
		if !errors.Is(err, os.ErrNotExist) {
			reportError(err)
		}
	}
	if stats.SwapLimit != nil && *stats.SwapLimit >= memoryUnlimitedValue {
		stats.SwapLimit = nil
	}

	return nil
}
