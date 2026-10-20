// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin || windows

package eudm

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
	"github.com/stretchr/testify/require"
)

func TestGetMissingIdentity(t *testing.T) {
	old := collectSystemInfo
	t.Cleanup(func() { collectSystemInfo = old })
	collectSystemInfo = func() (*systeminfo.SystemInfo, error) { return nil, errors.New("unavailable") }
	_, err := Get()
	require.Error(t, err)
	collectSystemInfo = func() (*systeminfo.SystemInfo, error) { return nil, nil }
	_, err = Get()
	require.Error(t, err)
	collectSystemInfo = func() (*systeminfo.SystemInfo, error) {
		return &systeminfo.SystemInfo{ComputerName: "device", SerialNumber: "Unknown"}, nil
	}
	_, err = Get()
	require.Error(t, err)
}
