// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package start

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	configstreamtestutil "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/testutil"
	pidimpl "github.com/DataDog/datadog-agent/comp/core/pid/impl"
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	tests := []struct {
		name     string
		cliInput []string
		check    func(pidParams pidimpl.Params, params core.BundleParams)
	}{
		{
			name:     "start",
			cliInput: []string{"start"},
			check: func(_ pidimpl.Params, params core.BundleParams) {
				// Verify logger defaults
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
			},
		},
		{
			name:     "pidfile",
			cliInput: []string{"start", "--pidfile", "/pid/file"},
			check: func(pidParams pidimpl.Params, params core.BundleParams) {
				// Verify logger defaults
				require.Equal(t, command.LoggerName, params.LoggerName(), "logger name not matching")
				require.Equal(t, "/pid/file", pidParams.PIDfilePath, "PID file path not matching")
			},
		},
	}

	// configstream is on by default, so the fx graph blocks until it seeds from a core agent.
	configstreambootstrap.UseDynamicSchema(t)
	fakeCore := configstreamtestutil.StartFakeCoreAgent(t, t.TempDir())

	for _, test := range tests {
		fxutil.TestOneShotSubcommand(t,
			Commands(newGlobalParamsTest(t, fakeCore)),
			test.cliInput,
			start,
			test.check,
		)
	}

	registers, _ := fakeCore.Counts()
	require.NotZero(t, registers, "consumer never registered, so this test no longer covers configstream")
}

func newGlobalParamsTest(t *testing.T, fakeCore *configstreamtestutil.FakeCoreAgent) *command.GlobalParams {
	// the config needs an existing config file when initializing
	config := path.Join(t.TempDir(), "datadog.yaml")
	err := os.WriteFile(config, []byte("hostname: test\n"+fakeCore.ConfigYAML()), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfigFilePaths: []string{config},
	}
}
