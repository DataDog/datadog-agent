// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestProcessNameExtractorExtract(t *testing.T) {
	e := NewProcessNameExtractor()

	procs := map[int32]*procutil.Process{
		1: {Pid: 1, Comm: "nginx"},
		2: {Pid: 2, Comm: "python3"},
		3: {Pid: 3, Comm: "", Exe: "/usr/bin/myapp"},
		4: {Pid: 4, Comm: "", Exe: ""},
	}
	e.Extract(procs)

	assert.Equal(t, "nginx", e.GetProcessName(1))
	assert.Equal(t, "python3", e.GetProcessName(2))
	assert.Equal(t, "myapp", e.GetProcessName(3), "falls back to executable name when Comm is empty")
	assert.Equal(t, "", e.GetProcessName(4), "returns empty string when both Comm and Exe are empty")
	assert.Equal(t, "", e.GetProcessName(99), "unknown pid returns empty string")
}

func TestProcessNameExtractorBeforeExtract(t *testing.T) {
	e := NewProcessNameExtractor()
	assert.Equal(t, "", e.GetProcessName(1), "returns empty string before any Extract call")
}

func TestProcessNameExtractorExtractReplacesStaleData(t *testing.T) {
	e := NewProcessNameExtractor()

	e.Extract(map[int32]*procutil.Process{
		1: {Pid: 1, Comm: "oldname"},
	})
	assert.Equal(t, "oldname", e.GetProcessName(1))

	// Second Extract with a new process list — pid 1 is gone, pid 2 is new
	e.Extract(map[int32]*procutil.Process{
		2: {Pid: 2, Comm: "newname"},
	})
	assert.Equal(t, "", e.GetProcessName(1), "stale pid should be gone after re-extract")
	assert.Equal(t, "newname", e.GetProcessName(2))
}
