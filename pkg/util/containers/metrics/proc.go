// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build linux

package metrics

import (
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// hostProcFunc allows hostProc to be overridden for ease of mock testing
var hostProcFunc func(...string) string = func(combineWith ...string) string {
	return hostProc(combineWith...)
}

// GetFileDescriptorLen gets the number of open file descriptors for a given pid
func GetFileDescriptorLen(pid int) (int, error) {
	// Open proc file descriptor dir
	fdPath := hostProcFunc(strconv.Itoa(pid), "fd")
	d, err := os.Open(fdPath)
	if err != nil {
		return 0, err
	}
	defer d.Close()

	// Get all file names
	names, err := d.Readdirnames(-1)
	if err != nil {
		return 0, log.Warnf("Could not read %s: %s", d.Name(), err)
	}

	return len(names), nil
}
