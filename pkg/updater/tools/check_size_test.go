// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tools contains tooling required by the updater.
package tools

import (
	"testing"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/stretchr/testify/assert"
)

func TestCheckAvailableDiskSpace(t *testing.T) {
	tmpDir := t.TempDir()

	// Get the tmpDir partition size
	s, err := disk.Usage(tmpDir)
	assert.NoError(t, err)

	// Check if the tmpDir partition has enough space to store 0 bytes
	enough, err := CheckAvailableDiskSpace(tmpDir, 0)
	assert.NoError(t, err)
	assert.True(t, enough)

	// Check if the tmpDir partition has enough space to store 1 byte
	enough, err = CheckAvailableDiskSpace(tmpDir, 1)
	assert.NoError(t, err)
	assert.True(t, enough)

	// Check if the tmpDir partition has enough space to store the entire partition
	enough, err = CheckAvailableDiskSpace(tmpDir, s.Free)
	assert.NoError(t, err)
	assert.True(t, enough)

	// Check if the tmpDir partition has enough space to store the entire partition + 1 byte
	enough, err = CheckAvailableDiskSpace(tmpDir, s.Free+1)
	assert.NoError(t, err)
	assert.False(t, enough)

	// Check that a non existing dir can't be tested
	enough, err = CheckAvailableDiskSpace("/non-existing-dir", 1)
	assert.Error(t, err)
	assert.False(t, enough)
}
