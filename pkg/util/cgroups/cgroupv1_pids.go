// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package cgroups

import "strconv"

func (c *cgroupV1) GetPIDStats(stats *PIDStats) error {
	if stats == nil {
		return &InvalidInputError{Desc: "input stats cannot be nil"}
	}

	if !c.controllerMounted("pids") {
		return &ControllerNotFoundError{Controller: "pids"}
	}

	stats.PIDs = nil
	if err := parseFile(c.fr, c.pathFor("pids", "cgroup.procs"), func(s string) error {
		pid, err := strconv.Atoi(s)
		if err != nil {
			reportError(newValueError(s, err))
			return nil
		}

		stats.PIDs = append(stats.PIDs, pid)

		return nil
	}); err != nil {
		reportError(err)
	}

	// In pids.current we get count of TIDs+PIDs
	if err := parseSingleUnsignedStat(c.fr, c.pathFor("pids", "pids.current"), &stats.HierarchicalThreadCount); err != nil {
		reportError(err)
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("pids", "pids.max"), &stats.HierarchicalThreadLimit); err != nil {
		reportError(err)
	}

	return nil
}
