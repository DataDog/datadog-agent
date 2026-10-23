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
	got, err := Get()
	require.NoError(t, err)
	require.Equal(t, "my-pc-abc123", got)
	osHostname = func() (string, error) { return "", errors.New("unavailable") }
	_, err = Get()
	require.Error(t, err)
}
