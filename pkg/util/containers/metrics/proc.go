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

// Define hostProcFunc for ease of mock testing
var hostProcFunc func(...string) string = func(stuff ...string) string {
	return hostProc(stuff...)
}

func GetFileDescriptorLen(pid int) (int, error) {
	// Open proc file descriptor dir
	fdPath := hostProcFunc(strconv.Itoa(pid), "fd")
	d, err := os.Open(fdPath)
	if err != nil {
		return 0, err
	}
	defer d.Close()

	names, err := d.Readdirnames(-1)
	if err != nil {
		return 0, log.Warnf("Could not read %s: %s", d.Name(), err)
	}

	return len(names), nil
}
