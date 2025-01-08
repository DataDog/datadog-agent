// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package debug contains handlers for debug information global to all of system-probe
package debug

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const userFacility = 8

func TestKlogLevelName(t *testing.T) {
	require.Equal(t, "emerg", klogLevelName(0))
	require.Equal(t, "notice", klogLevelName(5))

	require.Equal(t, "notice", klogLevelName(userFacility|5))
}
