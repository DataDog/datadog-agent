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

// driverCollectorFor builds a collector backed by fixture records.
func driverCollectorFor(records []win32PnPSignedDriver) *driverCollector {
	return &driverCollector{
		queryFn: func() ([]win32PnPSignedDriver, error) { return records, nil },
	}
}

func TestDriverCollectorGroupsPerPackage(t *testing.T) {
	// The same package bound to three devices must collapse into a single entry,
	// carrying the highest version of the group.
	collector := driverCollectorFor([]win32PnPSignedDriver{
		{InfName: "oem12.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Wi-Fi Adapter", DriverVersion: "22.10.0.5"},
		{InfName: "oem12.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Wi-Fi Adapter", DriverVersion: "22.9.0.1"},
		{InfName: "oem12.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Wi-Fi Adapter", DriverVersion: "22.100.0.2"},
	})

	entries, warnings, err := collector.Collect()

	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.Len(t, entries, 1)
	assert.Equal(t, "22.100.0.2", entries[0].Version, "the highest version in the group should win")
	assert.Equal(t, softwareTypeDriver, entries[0].Source)
	assert.Equal(t, "Wi-Fi Adapter", entries[0].DisplayName)
	assert.Equal(t, "Intel", entries[0].Publisher)
	assert.Equal(t, "installed", entries[0].Status)
	assert.Empty(t, entries[0].InstallDate, "DriverDate is a build date, not an install time")
}

func TestDriverCollectorGroupsMultiModelPackage(t *testing.T) {
	// One INF commonly supports several device models, and each binding reports its own
	// description. Grouping on the description would emit the same installed package
	// once per model, so the INF name is what defines the package here.
	collector := driverCollectorFor([]win32PnPSignedDriver{
		{InfName: "oem12.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Intel(R) Wi-Fi 6 AX201", DriverVersion: "22.10.0.5"},
		{InfName: "oem12.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Intel(R) Wi-Fi 6 AX200", DriverVersion: "22.100.0.2"},
	})

	entries, _, err := collector.Collect()

	require.NoError(t, err)
	require.Len(t, entries, 1, "one INF package is one entry regardless of how many models it binds")
	assert.Equal(t, "22.100.0.2", entries[0].Version)
	// The representative is the lowest description, so identity does not depend on the
	// order WMI returned the bindings in.
	assert.Equal(t, "Intel(R) Wi-Fi 6 AX200", entries[0].DisplayName)
}

func TestDriverCollectorSeparatesDistinctInfPackages(t *testing.T) {
	collector := driverCollectorFor([]win32PnPSignedDriver{
		{InfName: "oem12.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Intel(R) Wi-Fi 6 AX201", DriverVersion: "22.10.0.5"},
		{InfName: "oem13.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Intel(R) Ethernet I219-V", DriverVersion: "12.19.1.34"},
	})

	entries, _, err := collector.Collect()

	require.NoError(t, err)
	require.Len(t, entries, 2, "two INF packages stay two entries")

	ids := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		ids[entry.GetID()] = struct{}{}
	}
	assert.Len(t, ids, 2, "entry IDs must stay unique")
}

func TestDriverCollectorMergesCollidingProductCodes(t *testing.T) {
	// Two INF packages that agree on provider, class and description would otherwise
	// produce the same product code, and therefore duplicate entry IDs.
	collector := driverCollectorFor([]win32PnPSignedDriver{
		{InfName: "oem12.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Wi-Fi Adapter", DriverVersion: "22.10.0.5"},
		{InfName: "oem45.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Wi-Fi Adapter", DriverVersion: "22.100.0.2"},
	})

	entries, _, err := collector.Collect()

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "22.100.0.2", entries[0].Version, "the highest version wins on collision")
}

func TestDriverCollectorScopesToOEMPackages(t *testing.T) {
	collector := driverCollectorFor([]win32PnPSignedDriver{
		{InfName: "oem12.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Wi-Fi Adapter", DriverVersion: "1.0"},
		{InfName: "OEM7.INF", DriverProviderName: "NVIDIA", DeviceClass: "Display", Description: "GPU", DriverVersion: "2.0"},
		{InfName: "netwtw10.inf", DriverProviderName: "Microsoft", DeviceClass: "Net", Description: "Inbox NIC", DriverVersion: "3.0"},
		{InfName: "", DriverProviderName: "Microsoft", DeviceClass: "System", Description: "Unbound", DriverVersion: "4.0"},
	})

	entries, _, err := collector.Collect()

	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.DisplayName)
	}
	assert.ElementsMatch(t, []string{"Wi-Fi Adapter", "GPU"}, names,
		"only oemNN.inf packages are in scope, case-insensitively")
}

func TestDriverIdentitySurvivesVersionBump(t *testing.T) {
	// Windows reassigns the oemNN number when a package is republished. If identity
	// depended on InfName, an update would look like an uninstall plus a new install.
	before, _, err := driverCollectorFor([]win32PnPSignedDriver{
		{InfName: "oem12.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Wi-Fi Adapter", DriverVersion: "22.9.0.1"},
	}).Collect()
	require.NoError(t, err)
	require.Len(t, before, 1)

	after, _, err := driverCollectorFor([]win32PnPSignedDriver{
		{InfName: "oem45.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Wi-Fi Adapter", DriverVersion: "23.0.0.0"},
	}).Collect()
	require.NoError(t, err)
	require.Len(t, after, 1)

	assert.Equal(t, before[0].ProductCode, after[0].ProductCode)
	assert.Equal(t, before[0].GetID(), after[0].GetID())
	assert.NotEqual(t, before[0].Version, after[0].Version)
}

func TestDriverCollectorSkipsIncompleteRecords(t *testing.T) {
	collector := driverCollectorFor([]win32PnPSignedDriver{
		{InfName: "oem1.inf", DriverProviderName: "", DeviceClass: "Net", Description: "No Provider", DriverVersion: "1.0"},
		{InfName: "oem2.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "", DriverVersion: "1.0"},
		{InfName: "oem3.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "No Version", DriverVersion: ""},
		{InfName: "oem4.inf", DriverProviderName: "Intel", DeviceClass: "Net", Description: "Good Driver", DriverVersion: "1.0"},
	})

	entries, warnings, err := collector.Collect()

	require.NoError(t, err, "bad records degrade to warnings, they do not fail the snapshot")
	assert.Len(t, warnings, 3)
	require.Len(t, entries, 1)
	assert.Equal(t, "Good Driver", entries[0].DisplayName)
}

func TestDriverCollectorQueryFailureIsFatal(t *testing.T) {
	collector := &driverCollector{
		queryFn: func() ([]win32PnPSignedDriver, error) { return nil, errors.New("WMI unavailable") },
	}

	entries, _, err := collector.Collect()

	assert.Error(t, err, "a failed enumeration is an unknown state, not an empty one")
	assert.Empty(t, entries)
}

func TestDriverCollectorEmptyResultIsNotAnError(t *testing.T) {
	entries, warnings, err := driverCollectorFor(nil).Collect()

	assert.NoError(t, err)
	assert.Empty(t, entries)
	assert.Empty(t, warnings)
}
