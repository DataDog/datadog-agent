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
	require.Equal(t, "My-Mac-abc123", got)
	info.ComputerName = "Renamed Mac"
	renamed, err := Get(true)
	require.NoError(t, err)
	require.Equal(t, "Renamed-Mac-abc123", renamed)
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
			require.Equal(t, "My-Mac", got)
		})
	}
}

func TestGetComputerNameApostrophes(t *testing.T) {
	old := collectSystemInfo
	t.Cleanup(func() { collectSystemInfo = old })
	for _, name := range []string{"Jack's MacBook", "Jack’s MacBook", "'Jack’s' MacBook’"} {
		t.Run(name, func(t *testing.T) {
			collectSystemInfo = func() (*systeminfo.SystemInfo, error) {
				return &systeminfo.SystemInfo{ComputerName: name, SerialNumber: "ABC123"}, nil
			}
			got, err := Get(false)
			require.NoError(t, err)
			require.Equal(t, "Jacks-MacBook", got)

			got, err = Get(true)
			require.NoError(t, err)
			require.Equal(t, "Jacks-MacBook-abc123", got)
		})
	}
	collectSystemInfo = func() (*systeminfo.SystemInfo, error) {
		return &systeminfo.SystemInfo{ComputerName: "'’"}, nil
	}
	_, err := Get(false)
	require.Error(t, err)
}
