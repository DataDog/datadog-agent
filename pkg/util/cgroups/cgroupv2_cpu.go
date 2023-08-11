// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"bytes"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func (c *cgroupV2) GetCPUStats(stats *CPUStats) error {
	if stats == nil {
		return &InvalidInputError{Desc: "input stats cannot be nil"}
	}

	if !c.controllerActivated("cpu") {
		return &ControllerNotFoundError{Controller: "cpu"}
	}

	c.parseCPUController(stats)

	// Do not raise error if `cpuset` is not present as it's not used to retrieve key features
	if c.controllerActivated("cpuset") {
		c.parseCPUSetController(stats)
	}

	return nil
}

func (c *cgroupV2) parseCPUController(stats *CPUStats) {
	if err := parse2ColumnStats(c.fr, c.pathFor("cpu.stat"), 0, 1, parseV2CPUStat(stats)); err != nil {
		reportError(err)
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("cpu.weight"), &stats.Shares); err != nil {
		reportError(err)
	}

	if err := parse2ColumnStats(c.fr, c.pathFor("cpu.max"), 0, 1, parseV2CPUMax(stats)); err != nil {
		reportError(err)
	}

	if err := parsePSI(c.fr, c.pathFor("cpu.pressure"), &stats.PSISome, nil); err != nil {
		reportError(err)
	}
}

func (c *cgroupV2) parseCPUSetController(stats *CPUStats) {
	// Normally there's only one line, but as the parser works line by line anyway, we do support multiple lines
	var cpuCount uint64
	err := parseFile(c.fr, c.pathFor("cpuset.cpus.effective"), func(line []byte) error {
		cpuCount += ParseCPUSetFormat(line)
		return nil
	})

	if err != nil {
		reportError(err)
	} else if cpuCount != 0 {
		stats.CPUCount = &cpuCount
	}
}

func parseV2CPUStat(stats *CPUStats) func(key, value []byte) error {
	return func(key, value []byte) error {
		// Do not stop parsing the file if we cannot parse a single value
		intVal, err := strconv.ParseUint(string(value), 10, 64)
		if err != nil {
			reportError(newValueError(string(value), err))
			return nil
		}

		if bytes.Equal(key, []byte("usage_usec")) {
			intVal *= uint64(time.Microsecond)
			stats.Total = &intVal
		}
		if bytes.Equal(key, []byte("user_usec")) {
			intVal *= uint64(time.Microsecond)
			stats.User = &intVal
		}
		if bytes.Equal(key, []byte("system_usec")) {
			intVal *= uint64(time.Microsecond)
			stats.System = &intVal
		}
		if bytes.Equal(key, []byte("nr_periods")) {
			stats.ElapsedPeriods = &intVal
		}
		if bytes.Equal(key, []byte("nr_throttled")) {
			stats.ThrottledPeriods = &intVal
		}
		if bytes.Equal(key, []byte("throttled_usec")) {
			intVal *= uint64(time.Microsecond)
			stats.ThrottledTime = &intVal
		}

		return nil
	}
}

func parseV2CPUMax(stats *CPUStats) func(key, value []byte) error {
	return func(limit, period []byte) error {
		periodVal, err := strconv.ParseUint(string(period), 10, 64)
		if err == nil {
			stats.SchedulerPeriod = pointer.Ptr(periodVal * uint64(time.Microsecond))
		} else {
			return newValueError(string(period), err)
		}

		if !bytes.Equal(limit, []byte("max")) {
			limitVal, err := strconv.ParseUint(string(limit), 10, 64)
			if err == nil {
				stats.SchedulerQuota = pointer.Ptr(limitVal * uint64(time.Microsecond))
			} else {
				return newValueError(string(limit), err)
			}
		}

		return nil
	}
}
