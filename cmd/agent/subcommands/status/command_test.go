// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStatusCommand(t *testing.T) {
	defer os.Unsetenv("DD_AUTOCONFIG_FROM_ENVIRONMENT") // undo os.Setenv by RunE
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"status", "-j"},
		statusCmd,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Empty(t, cliParams.args)
			require.Equal(t, true, cliParams.jsonStatus)
		})
}

func TestComponentStatusCommand(t *testing.T) {
	defer os.Unsetenv("DD_AUTOCONFIG_FROM_ENVIRONMENT") // undo os.Setenv by RunE
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"status", "component", "abc"},
		statusCmd,
		func(cliParams *cliParams, _ core.BundleParams) {
			require.Equal(t, []string{"component", "abc"}, cliParams.args)
			require.Equal(t, false, cliParams.jsonStatus)
		})
}

func TestStatusCommandLogLevelFromEnv(t *testing.T) {
	defer os.Unsetenv("DD_AUTOCONFIG_FROM_ENVIRONMENT") // undo os.Setenv by RunE
	t.Setenv("DD_LOG_LEVEL", "info")

	tests := []struct {
		args          []string
		expectedLevel string
	}{
		{[]string{"status"}, "info"},
		{[]string{"status", "-j"}, "off"},
		{[]string{"status", "-p"}, "off"},
		{[]string{"status", "-j", "--log_level", "debug"}, "debug"},
	}
	for _, tt := range tests {
		fxutil.TestOneShotSubcommand(t,
			Commands(&command.GlobalParams{}),
			tt.args,
			statusCmd,
			func(_ *cliParams, params core.BundleParams) {
				require.Equal(t, tt.expectedLevel, params.LogParams.LogLevelFn(nil), "args: %v", tt.args)
			})
	}
}
