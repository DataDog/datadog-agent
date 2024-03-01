// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tools contains tooling required by the updater.
package tools

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/stretchr/testify/assert"
)

type mockDisk struct {
	total     uint64
	available uint64
}

func (d mockDisk) GetUsage(path string) (*filesystem.DiskUsage, error) {
	return &filesystem.DiskUsage{
		Total:     d.total,
		Available: d.available,
	}, nil
}

func TestCheckAvailableDiskSpace(t *testing.T) {
	fakeDisk := mockDisk{
		total:     100,
		available: 50,
	}
	// Check if the partition has enough space to store 0 bytes
	enough, err := CheckAvailableDiskSpace(fakeDisk, "/", 0)
	assert.NoError(t, err)
	assert.True(t, enough)

	// Check if the partition has enough space to store 1 byte
	enough, err = CheckAvailableDiskSpace(fakeDisk, "/", 1)
	assert.NoError(t, err)
	assert.True(t, enough)

	// Check if the partition has enough space to store the entire partition
	enough, err = CheckAvailableDiskSpace(fakeDisk, "/", 50)
	assert.NoError(t, err)
	assert.True(t, enough)

	// Check if the partition has enough space to store the entire partition + 1 byte
	enough, err = CheckAvailableDiskSpace(fakeDisk, "/", 51)
	assert.NoError(t, err)
	assert.False(t, enough)
}
