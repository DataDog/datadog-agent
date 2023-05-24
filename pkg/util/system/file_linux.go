// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CountProcessesFileDescriptors returns the sum of open file descriptors for all given PIDs.
// Failed PIDs are silently skipped.
// A boolean is returned to indicate whether all PIDs failed or not.
func CountProcessesFileDescriptors(procPath string, pids []int) (uint64, bool) {
	// Compute the number of open FDs
	allErrors := true
	var fdSum int
	for _, pid := range pids {
		fdsPerPid, err := CountProcessFileDescriptors(procPath, pid)
		if err != nil {
			log.Tracef("Unable to get number of FDs for pid: %d", pid)
		} else {
			allErrors = false
			fdSum += fdsPerPid
		}
	}

	return uint64(fdSum), allErrors
}

// CountProcessFileDescriptors gets the number of open file descriptors for a given pid
func CountProcessFileDescriptors(procPath string, pid int) (int, error) {
	// Open proc file descriptor dir
	fdPath := filepath.Join(procPath, strconv.Itoa(pid), "fd")
	d, err := os.Open(fdPath)
	if err != nil {
		return 0, err
	}
	defer d.Close()

	// Get all file names
	names, err := d.Readdirnames(-1)
	if err != nil {
		return 0, err
	}

	return len(names), nil
}
