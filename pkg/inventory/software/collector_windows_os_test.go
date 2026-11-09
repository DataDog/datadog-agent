// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// osCollectorFor builds a collector that reports a fixed version, so the assertions do
// not depend on how the host running the test is patched.
func osCollectorFor(version string) *osCollector {
	return &osCollector{
		versionFn: func() (string, error) { return version, nil },
	}
}

func TestOSCollectorReportsTheRunningSystem(t *testing.T) {
	entries, warnings, err := osCollectorFor("10.0.26100.4652").Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, entries, 1, "the operating system is exactly one entry")

	entry := entries[0]
	assert.Equal(t, softwareTypeOS, entry.Source)
	assert.Equal(t, "OS", entry.DisplayName)
	assert.Equal(t, "os_version", entry.ProductCode)
	assert.Equal(t, "10.0.26100.4652", entry.Version, "the revision is what distinguishes patch levels")
	assert.Equal(t, microsoftPublisher, entry.Publisher)
	assert.Equal(t, "installed", entry.Status)
	assert.Empty(t, entry.InstallDate, "the recorded install time is the original installation")
}

func TestOSCollectorIdentitySurvivesAnUpdate(t *testing.T) {
	// The backend keys a software item on name and product code with the version
	// excluded, so a cumulative update must change only the version. Anything
	// version-derived in the name or product code would read as an uninstall plus a
	// fresh install.
	before, _, err := osCollectorFor("10.0.26100.1742").Collect()
	require.NoError(t, err)
	require.Len(t, before, 1)

	after, _, err := osCollectorFor("10.0.26100.4652").Collect()
	require.NoError(t, err)
	require.Len(t, after, 1)

	assert.Equal(t, before[0].DisplayName, after[0].DisplayName)
	assert.Equal(t, before[0].ProductCode, after[0].ProductCode)
	assert.Equal(t, before[0].GetID(), after[0].GetID())
	assert.NotEqual(t, before[0].Version, after[0].Version)
}

func TestOSCollectorVersionFailureIsFatal(t *testing.T) {
	// A host always runs some version of Windows, so reporting nothing would read as the
	// operating system having been removed.
	collector := &osCollector{
		versionFn: func() (string, error) { return "", errors.New("registry unavailable") },
	}

	entries, _, err := collector.Collect()

	assert.Error(t, err)
	assert.Empty(t, entries)
}
