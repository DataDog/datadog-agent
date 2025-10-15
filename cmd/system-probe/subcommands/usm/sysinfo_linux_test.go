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
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestSysinfoCommand(t *testing.T) {
	globalParams := &command.GlobalParams{}
	cmd := makeSysinfoCommand(globalParams)

	require.NotNil(t, cmd)
	require.Equal(t, "sysinfo", cmd.Use)
	require.Equal(t, "Show system information relevant to USM", cmd.Short)

	// Test the OneShot command
	fxutil.TestOneShotSubcommand(t,
		Commands(globalParams),
		[]string{"usm", "sysinfo"},
		runSysinfo,
		func() {})
}
