// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package cgroups

func (c *cgroupV2) GetMemoryStats(stats *MemoryStats) error {
	if stats == nil {
		return &InvalidInputError{Desc: "input stats cannot be nil"}
	}

	if !c.controllerActivated("memory") {
		return &ControllerNotFoundError{Controller: "memory"}
	}

	var kernelStack, slab *uint64
	if err := parse2ColumnStatsWithMapping(c.fr, c.pathFor("memory.stat"), 0, 1, map[string]**uint64{
		"file":          &stats.Cache,
		"anon":          &stats.RSS,
		"anon_thp":      &stats.RSSHuge,
		"file_mapped":   &stats.MappedFile,
		"pgfault":       &stats.Pgfault,
		"pgmajfault":    &stats.Pgmajfault,
		"inactive_anon": &stats.InactiveAnon,
		"active_anon":   &stats.ActiveAnon,
		"inactive_file": &stats.InactiveFile,
		"active_file":   &stats.ActiveFile,
		"unevictable":   &stats.Unevictable,
		"kernel_stack":  &kernelStack,
		"slab":          &slab,
	}); err != nil {
		reportError(err)
	}

	if kernelStack != nil && slab != nil {
		stats.KernelMemory = uint64Ptr(*kernelStack + *slab)
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory.current"), &stats.UsageTotal); err != nil {
		reportError(err)
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory.min"), &stats.MinThreshold); err != nil {
		reportError(err)
	}
	nilIfZero(&stats.MinThreshold)

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory.low"), &stats.LowThreshold); err != nil {
		reportError(err)
	}
	nilIfZero(&stats.LowThreshold)

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory.high"), &stats.HighThreshold); err != nil {
		reportError(err)
	}
	nilIfZero(&stats.HighThreshold)

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory.max"), &stats.Limit); err != nil {
		reportError(err)
	}
	nilIfZero(&stats.Limit)

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory.swap.current"), &stats.Swap); err != nil {
		reportError(err)
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory.swap.high"), &stats.SwapHighThreshold); err != nil {
		reportError(err)
	}
	nilIfZero(&stats.SwapHighThreshold)

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("memory.swap.max"), &stats.SwapLimit); err != nil {
		reportError(err)
	}
	nilIfZero(&stats.SwapLimit)

	if err := parse2ColumnStatsWithMapping(c.fr, c.pathFor("memory.events"), 0, 1, map[string]**uint64{
		"oom":      &stats.OOMEvents,
		"oom_kill": &stats.OOMKiilEvents,
	}); err != nil {
		reportError(err)
	}

	if err := parsePSI(c.fr, c.pathFor("memory.pressure"), &stats.PSISome, &stats.PSIFull); err != nil {
		reportError(err)
	}

	return nil
}
