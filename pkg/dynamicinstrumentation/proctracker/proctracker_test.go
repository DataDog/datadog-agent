// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package proctracker

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessNotAdded(t *testing.T) {
	pt := ProcessTracker{
		processes: make(map[pid]binaryID),
	}

	// Use the current process ID for testing
	pid := uint32(os.Getpid())

	// Simulate process start
	pt.inspectBinary("", pid)

	// Ensure the process ID is not added to pt.processes
	pt.lock.RLock()
	defer pt.lock.RUnlock()
	_, exists := pt.processes[pid]
	assert.False(t, exists, "process ID should not be added to pt.processes")
}
