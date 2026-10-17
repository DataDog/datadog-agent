// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnumDriverServices verifies against the real host that the SCM enumeration finds every
// kernel-mode and file-system driver service, with an image path for most of them.
//
// "disk" is checked by name because every Windows host has it: it is the storage class driver
// and is present from the very first boot.
func TestEnumDriverServices(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	drivers, err := EnumDriverServices()
	require.NoError(t, err)
	require.NotEmpty(t, drivers, "a Windows host always has kernel driver services")

	var withImagePath int
	var foundDisk bool
	for _, driver := range drivers {
		assert.NotEmpty(t, driver.Name, "every service record has a name")
		if driver.ImagePath != "" {
			withImagePath++
		}
		if strings.EqualFold(driver.Name, "disk") {
			foundDisk = true
		}
	}

	assert.True(t, foundDisk, "the disk class driver is present on every Windows host")

	minimumWithImagePath := len(drivers) * 9 / 10
	assert.GreaterOrEqual(t, withImagePath, minimumWithImagePath,
		"at least 90%% of driver services should have a resolvable image path, got %d/%d",
		withImagePath, len(drivers))
}
