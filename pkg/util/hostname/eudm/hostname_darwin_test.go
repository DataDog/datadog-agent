// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eudm

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
	"github.com/stretchr/testify/require"
)

func TestGetUsesComputerName(t *testing.T) {
	old := collectSystemInfo
	t.Cleanup(func() { collectSystemInfo = old })
	info := &systeminfo.SystemInfo{ComputerName: "My Mac", SerialNumber: "ABC123"}
	collectSystemInfo = func() (*systeminfo.SystemInfo, error) { return info, nil }
	got, err := Get(true)
	require.NoError(t, err)
	require.Equal(t, "my-mac-abc123", got)
	info.ComputerName = "Renamed Mac"
	renamed, err := Get(true)
	require.NoError(t, err)
	require.Equal(t, "renamed-mac-abc123", renamed)
}

func TestGetComputerNameWithoutSerial(t *testing.T) {
	old := collectSystemInfo
	t.Cleanup(func() { collectSystemInfo = old })
	for _, serial := range []string{"ABC123", "", "Unknown"} {
		t.Run(serial, func(t *testing.T) {
			collectSystemInfo = func() (*systeminfo.SystemInfo, error) {
				return &systeminfo.SystemInfo{ComputerName: "My Mac", SerialNumber: serial}, nil
			}
			got, err := Get(false)
			require.NoError(t, err)
			require.Equal(t, "my-mac", got)
		})
	}
}
