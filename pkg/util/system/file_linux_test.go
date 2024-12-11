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
	"testing"

	"github.com/stretchr/testify/assert"
)

func createFakeFDs(rootPath string, pid, numFDs int) error {
	fdRootPath := filepath.Join(rootPath, strconv.Itoa(pid), "fd")
	if err := os.MkdirAll(fdRootPath, 0o755); err != nil {
		return err
	}

	for i := 0; i < numFDs; i++ {
		if err := os.MkdirAll(filepath.Join(fdRootPath, strconv.Itoa(i)), 0o755); err != nil {
			return err
		}
	}

	return nil
}

func TestCountProcessesFileDescriptors(t *testing.T) {
	fakeProc := t.TempDir()
	assert.NoError(t, createFakeFDs(fakeProc, 42, 1))
	assert.NoError(t, createFakeFDs(fakeProc, 421, 3))
	assert.NoError(t, createFakeFDs(fakeProc, 422, 8))

	count, allFailed := CountProcessesFileDescriptors(fakeProc, []int{42, 8, 421, 422})
	assert.EqualValues(t, 12, count)
	assert.False(t, allFailed)

	count, allFailed = CountProcessesFileDescriptors(fakeProc, []int{1, 2, 3})
	assert.EqualValues(t, 0, count)
	assert.True(t, allFailed)
}
