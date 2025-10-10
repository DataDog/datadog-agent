// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package usm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestConfigCommand(t *testing.T) {
	globalParams := &command.GlobalParams{}
	cmd := makeConfigCommand(globalParams)

	require.NotNil(t, cmd)
	require.Equal(t, "config", cmd.Use)
	require.Equal(t, "Show Universal Service Monitoring configuration", cmd.Short)

	// Test the OneShot command
	fxutil.TestOneShotSubcommand(t,
		Commands(globalParams),
		[]string{"usm", "config"},
		runConfig,
		func() {})
}
