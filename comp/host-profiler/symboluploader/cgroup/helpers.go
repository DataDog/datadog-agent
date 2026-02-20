// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cgroup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func isCgroup2UnifiedMode() bool {
	var st unix.Statfs_t
	err := unix.Statfs(cgroupRoot, &st)
	if err != nil {
		return false
	}
	return st.Type == unix.CGROUP2_SUPER_MAGIC
}

func readFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func v1GetCurrentCgroupPath() (string, error) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		// hierarchy:controller(s):path
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}

		for controller := range strings.SplitSeq(parts[1], ",") {
			if controller == "memory" {
				return filepath.Join(cgroupRoot, "memory", parts[2]), nil
			}
		}
	}

	return "", errors.New("cgroup path not found in /proc/self/cgroup")
}

func v2GetCurrentCgroupPath() (string, error) {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		if path, ok := strings.CutPrefix(line, "0::"); ok {
			return filepath.Join(cgroupRoot, path), nil
		}
	}

	return "", errors.New("cgroup path not found in /proc/self/cgroup")
}
