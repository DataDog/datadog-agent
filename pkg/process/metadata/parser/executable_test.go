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

func TestExecutableExtractorExtract(t *testing.T) {
	e := NewExecutableExtractor()

	procs := map[int32]*procutil.Process{
		1: {Pid: 1, Name: "nginx"},
		2: {Pid: 2, Name: "python3"},
		3: {Pid: 3, Name: ""},
	}
	e.Extract(procs)

	assert.Equal(t, "nginx", e.GetExecutableName(1))
	assert.Equal(t, "python3", e.GetExecutableName(2))
	assert.Equal(t, "", e.GetExecutableName(3))
	assert.Equal(t, "", e.GetExecutableName(99), "unknown pid returns empty string")
}

func TestExecutableExtractorBeforeExtract(t *testing.T) {
	e := NewExecutableExtractor()
	assert.Equal(t, "", e.GetExecutableName(1), "returns empty string before any Extract call")
}

func TestExecutableExtractorExtractReplacesStaleData(t *testing.T) {
	e := NewExecutableExtractor()

	e.Extract(map[int32]*procutil.Process{
		1: {Pid: 1, Name: "oldname"},
	})
	assert.Equal(t, "oldname", e.GetExecutableName(1))

	// Second Extract with a new process list — pid 1 is gone, pid 2 is new
	e.Extract(map[int32]*procutil.Process{
		2: {Pid: 2, Name: "newname"},
	})
	assert.Equal(t, "", e.GetExecutableName(1), "stale pid should be gone after re-extract")
	assert.Equal(t, "newname", e.GetExecutableName(2))
}
