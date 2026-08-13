// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package windows

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStateName(t *testing.T) {
	require.Equal(t, "active", stateName(0))
	require.Equal(t, "disconnected", stateName(4))
	require.Equal(t, "unknown_99", stateName(99))
}

func TestWindowsTimestamp(t *testing.T) {
	require.Nil(t, windowsTimestamp(0))
	windowsEpoch := int64(116444736000000000)
	result := windowsTimestamp(windowsEpoch + int64(5*time.Second/(100*time.Nanosecond)))
	require.NotNil(t, result)
	require.Equal(t, time.Unix(5, 0).UTC(), *result)
}
