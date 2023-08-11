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

func (c *cgroupV1) GetCPUStats(stats *CPUStats) error {
	if stats == nil {
		return &InvalidInputError{Desc: "input stats cannot be nil"}
	}

	if !c.controllerMounted("cpuacct") {
		return &ControllerNotFoundError{Controller: "cpuacct"}
	}
	if !c.controllerMounted("cpu") {
		return &ControllerNotFoundError{Controller: "cpu"}
	}

	c.parseCPUAcctController(stats)
	c.parseCPUController(stats)

	// Do not raise error if `cpuset` is not present as it's not used to retrieve key features
	if c.controllerMounted("cpuset") {
		c.parseCPUSetController(stats)
	}

	return nil
}

func (c *cgroupV1) parseCPUController(stats *CPUStats) {
	if err := parseSingleUnsignedStat(c.fr, c.pathFor("cpu", "cpu.shares"), &stats.Shares); err != nil {
		reportError(err)
	}

	if err := parse2ColumnStats(c.fr, c.pathFor("cpu", "cpu.stat"), 0, 1, func(key, value []byte) error {
		intVal, err := strconv.ParseUint(string(value), 10, 64)
		if err != nil {
			reportError(newValueError(string(value), err))
			// Dont't stop parsing on a single faulty value
			return nil
		}

		if bytes.Equal(key, []byte("nr_throttled")) {
			stats.ThrottledPeriods = &intVal
		}
		if bytes.Equal(key, []byte("throttled_time")) {
			stats.ThrottledTime = &intVal
		}
		if bytes.Equal(key, []byte("nr_periods")) {
			stats.ElapsedPeriods = &intVal
		}

		return nil
	}); err != nil {
		reportError(err)
	}

	if err := parseSingleUnsignedStat(c.fr, c.pathFor("cpu", "cpu.cfs_period_us"), &stats.SchedulerPeriod); err == nil {
		if stats.SchedulerPeriod != nil {
			stats.SchedulerPeriod = pointer.Ptr(*stats.SchedulerPeriod * uint64(time.Microsecond))
		}
	} else {
		reportError(err)
	}

	var tempValue *int64
	if err := parseSingleSignedStat(c.fr, c.pathFor("cpu", "cpu.cfs_quota_us"), &tempValue); err == nil {
		if tempValue != nil && *tempValue != -1 {
			stats.SchedulerQuota = pointer.Ptr(uint64(*tempValue) * uint64(time.Microsecond))
		}
	} else {
		reportError(err)
	}
}

func (c *cgroupV1) parseCPUAcctController(stats *CPUStats) {
	if err := parseSingleUnsignedStat(c.fr, c.pathFor("cpuacct", "cpuacct.usage"), &stats.Total); err != nil {
		reportError(err)
	}

	if err := parse2ColumnStats(c.fr, c.pathFor("cpuacct", "cpuacct.stat"), 0, 1, parseV1CPUAcctStatFn(stats)); err != nil {
		reportError(err)
	}
}

func (c *cgroupV1) parseCPUSetController(stats *CPUStats) {
	// Normally there's only one line, but as the parser works line by line anyway, we do support multiple lines
	var cpuCount uint64
	err := parseFile(c.fr, c.pathFor("cpuset", "cpuset.cpus"), func(line []byte) error {
		cpuCount += ParseCPUSetFormat(line)
		return nil
	})

	if err != nil {
		reportError(err)
	} else if cpuCount != 0 {
		stats.CPUCount = &cpuCount
	}
}

func parseV1CPUAcctStatFn(stats *CPUStats) func(key, val []byte) error {
	return func(key, val []byte) error {
		intVal, err := strconv.ParseUint(string(val), 10, 64)
		if err != nil {
			reportError(newValueError(string(val), err))
			return nil
		}

		switch string(key) {
		case "user":
			stats.User = pointer.Ptr(intVal * UserHZToNano)
		case "system":
			stats.System = pointer.Ptr(intVal * UserHZToNano)
		}

		return nil
	}
}
