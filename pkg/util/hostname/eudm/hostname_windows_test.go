// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eudm

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
	"github.com/stretchr/testify/require"
)

func TestGetUsesOSHostname(t *testing.T) {
	oldCollect, oldHostname := collectSystemInfo, osHostname
	t.Cleanup(func() { collectSystemInfo, osHostname = oldCollect, oldHostname })
	collectSystemInfo = func() (*systeminfo.SystemInfo, error) {
		return &systeminfo.SystemInfo{ComputerName: "ignored", SerialNumber: "ABC123"}, nil
	}
	osHostname = func() (string, error) { return "MY-PC", nil }
	got, err := Get(true)
	require.NoError(t, err)
	require.Equal(t, "MY-PC-abc123", got)
	osHostname = func() (string, error) { return "", errors.New("unavailable") }
	_, err = Get(true)
	require.Error(t, err)
}

func TestGetOSHostnameWithoutSerial(t *testing.T) {
	oldCollect, oldHostname := collectSystemInfo, osHostname
	t.Cleanup(func() { collectSystemInfo, osHostname = oldCollect, oldHostname })
	osHostname = func() (string, error) { return "MY-PC", nil }
	collectSystemInfo = func() (*systeminfo.SystemInfo, error) {
		t.Fatal("system information must not be collected when serial appending is disabled")
		return nil, errors.New("WMI unavailable")
	}
	got, err := Get(false)
	require.NoError(t, err)
	require.Equal(t, "MY-PC", got)

	osHostname = func() (string, error) { return "", errors.New("hostname unavailable") }
	_, err = Get(false)
	require.ErrorContains(t, err, "hostname unavailable")
}
