// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"bytes"
	"strconv"
)

const (
	cgroupProcsFile = "cgroup.procs"
	procCgroupFile  = "cgroup"
)

// ParseCPUSetFormat counts CPUs in CPUSet specs like "0,1,5-8". These are comma-separated lists
// of processor IDs, with hyphenated ranges representing closed sets.
// So "0,1,5-8" represents processors 0, 1, 5, 6, 7, 8.
// The function returns the count of CPUs, in this case 6.
func ParseCPUSetFormat(line []byte) uint64 {
	var numCPUs uint64

	var currentSegment []byte
	for len(line) != 0 {
		nextStart := bytes.IndexByte(line, ',')
		if nextStart == -1 {
			currentSegment = line
			line = nil
		} else {
			currentSegment = line[:nextStart]
			line = line[nextStart+1:]
		}

		if split := bytes.IndexByte(currentSegment, '-'); split != -1 {
			p0, _ := strconv.Atoi(string(currentSegment[:split]))
			p1, _ := strconv.Atoi(string(currentSegment[split+1:]))
			numCPUs += uint64(p1 - p0 + 1)
		} else {
			numCPUs += 1
		}
	}
	return numCPUs
}

func nilIfZero(value **uint64) {
	if *value != nil && **value == 0 {
		*value = nil
	}
}
