// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package cgroup

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// ContainerCgroup is a structure that stores paths and mounts for a cgroup.
// It provides several methods for collecting stats about the cgroup using the
// paths and mounts metadata.
type ContainerCgroup struct {
	ContainerID string
	Pids        []int32
	Paths       map[string]string
	Mounts      map[string]string
}

// readLines reads contents from a file and splits them by new lines.
func readLines(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return []string{""}, err
	}
	defer f.Close()

	var ret []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ret = append(ret, scanner.Text())
	}
	return ret, scanner.Err()
}

// hostProc returns the location of a host's procfs. This can and will be
// overridden when running inside a container.
func hostProc(combineWith ...string) string {
	parts := append([]string{config.Datadog.GetString("container_proc_root")}, combineWith...)
	return filepath.Join(parts...)
}

// pathExists returns a boolean indicating if the given path exists on the file system.
func pathExists(filename string) bool {
	if _, err := os.Stat(filename); err == nil {
		return true
	}
	return false
}

func parseCPUSetFile(lines []string) int {
	numCPUs := 0
	if len(lines) == 0 {
		return numCPUs
	}
	// File contents should be only one line so assume this is the case.
	line := lines[0]

	// Examples of List Format:
	//		0-5
	//		0-4,9
	//		0-2,7,12-14
	lineSlice := strings.Split(line, ",")
	for _, l := range lineSlice {
		lineParts := strings.Split(l, "-")
		if len(lineParts) == 2 {
			p0, _ := strconv.Atoi(lineParts[0])
			p1, _ := strconv.Atoi(lineParts[1])
			numCPUs += p1 - p0 + 1
		} else if len(lineParts) == 1 {
			numCPUs++
		}
	}

	return numCPUs
}
