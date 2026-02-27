// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package cgroup provides utilities for returning usable memory from cgroups.
package cgroup

import (
	"path/filepath"
	"strconv"
)

const (
	cgroupRoot     = "/sys/fs/cgroup"
	v1MaxMemory    = "memory.limit_in_bytes"
	v2MaxMemory    = "memory.max"
	memoryMaxUnset = 0x7FFFFFFFFFFFF000
	budgetRatio    = 8
)

func v1GetMaxUsableMemory() (int64, error) {
	cgroupPath, err := v1GetCurrentCgroupPath()
	if err != nil {
		return -1, err
	}

	str, err := readFromFile(filepath.Join(cgroupPath, v1MaxMemory))
	if err != nil {
		return -1, err
	}

	limit, err := strconv.ParseInt(str, 10, 64)

	if err != nil {
		return -1, err
	}

	if limit < 0 || limit == memoryMaxUnset {
		return -1, nil
	}

	return limit, nil
}

func v2GetMaxUsableMemory() (int64, error) {
	currCgroupPath, err := v2GetCurrentCgroupPath()
	if err != nil {
		return -1, err
	}

	str, err := readFromFile(filepath.Join(currCgroupPath, v2MaxMemory))

	if err != nil {
		return -1, err
	}

	// not error -> no memory constraints
	if str == "max" {
		return -1, nil
	}

	limit, err := strconv.ParseInt(str, 10, 64)

	if err != nil {
		return -1, err
	}

	return limit, nil
}

func GetMaxUsableMemory() (int64, error) {
	if !isCgroup2UnifiedMode() {
		return v1GetMaxUsableMemory()
	}
	return v2GetMaxUsableMemory()
}

// GetMemoryBudget returns a percentage of the overall memory limit found in
// cgroup. If there are no limits, it returns -1.
func GetMemoryBudget() (int64, error) {
	maxMemory, err := GetMaxUsableMemory()
	if err != nil {
		return -1, err
	}
	if maxMemory == -1 {
		return maxMemory, nil
	}

	return maxMemory * budgetRatio / 10, nil
}
