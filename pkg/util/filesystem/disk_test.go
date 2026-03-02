// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package filesystem

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDisk(t *testing.T) {
	disk := NewDisk()
	assert.NotNil(t, disk)
}

func TestDiskGetUsage(t *testing.T) {
	disk := NewDisk()

	// Get usage of the temp directory (should always exist)
	tmpDir := os.TempDir()
	usage, err := disk.GetUsage(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, usage)

	// Total should be greater than 0
	assert.Greater(t, usage.Total, uint64(0))
	// Available should be less than or equal to Total
	assert.LessOrEqual(t, usage.Available, usage.Total)
}

func TestDiskGetUsageCurrentDir(t *testing.T) {
	disk := NewDisk()

	// Get usage of current directory
	usage, err := disk.GetUsage(".")
	require.NoError(t, err)
	require.NotNil(t, usage)

	assert.Greater(t, usage.Total, uint64(0))
}

func TestDiskGetUsageRootDir(t *testing.T) {
	disk := NewDisk()

	// Get usage of root directory
	usage, err := disk.GetUsage("/")
	require.NoError(t, err)
	require.NotNil(t, usage)

	assert.Greater(t, usage.Total, uint64(0))
}

func TestDiskGetUsageInvalidPath(t *testing.T) {
	disk := NewDisk()

	// Get usage of non-existent path
	_, err := disk.GetUsage("/nonexistent/path/that/does/not/exist")
	assert.Error(t, err)
}
