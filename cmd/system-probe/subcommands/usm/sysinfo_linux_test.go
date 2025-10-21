// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package usm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
)

func TestSysinfoCommand(t *testing.T) {
	globalParams := &command.GlobalParams{}
	cmd := makeSysinfoCommand(globalParams)

	require.NotNil(t, cmd)
	require.Equal(t, "sysinfo", cmd.Use)
	require.Equal(t, "Show system information relevant to USM", cmd.Short)

	// Verify --max-cmdline-length flag exists
	maxCmdlineFlag := cmd.Flags().Lookup("max-cmdline-length")
	require.NotNil(t, maxCmdlineFlag, "--max-cmdline-length flag should exist")
	require.Equal(t, "50", maxCmdlineFlag.DefValue, "--max-cmdline-length should default to 50")

	// Verify --max-name-length flag exists
	maxNameFlag := cmd.Flags().Lookup("max-name-length")
	require.NotNil(t, maxNameFlag, "--max-name-length flag should exist")
	require.Equal(t, "25", maxNameFlag.DefValue, "--max-name-length should default to 25")
}
