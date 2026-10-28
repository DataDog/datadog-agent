// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	configstreamtestutil "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/testutil"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func prepareRunCommandTest(t *testing.T, coreAgentIPC bool) (string, *configstreamtestutil.FakeCoreAgent) {
	t.Helper()

	// Because fx.Invoke builds the ipc component, we need to ensure we
	// have a valid auth token before building the app for real.
	testDir := t.TempDir()

	// configstream is on by default, so the fx graph blocks until it seeds from a core agent.
	configstreambootstrap.UseDynamicSchema(t)
	fakeCore := configstreamtestutil.StartFakeCoreAgent(t, testDir)

	config := "hostname: test\n" + fakeCore.ConfigYAML()
	if !coreAgentIPC {
		// The command is pointed at this file for system-probe.yaml too.
		config += "system_probe_config:\n  core_agent_ipc:\n    enabled: false\n"
	}

	configPath := filepath.Join(testDir, "datadog.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0644))

	return configPath, fakeCore
}

func TestRunCommand(t *testing.T) {
	tests := []struct {
		name         string
		coreAgentIPC bool
	}{
		{
			name:         "default",
			coreAgentIPC: true,
		},
		{
			// With core agent IPC disabled every component that would talk to the
			// core agent is replaced by a local or no-op implementation, or left out.
			// TestOneShotSubcommand validates the graph and then actually starts it,
			// so this covers the whole fallback assembly. The fake core agent is
			// still reachable, so a zero registration count proves nothing reached it.
			name:         "core agent IPC disabled",
			coreAgentIPC: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configPath, fakeCore := prepareRunCommandTest(t, test.coreAgentIPC)

			configComponent.NewMockFromYAMLFile(t, configPath)

			fxutil.Test[ipc.Component](t,
				ipcfx.ModuleReadWrite(),
				core.MockBundle(),
			)

			fxutil.TestOneShotSubcommand(t,
				Commands(&command.GlobalParams{
					ConfFilePath: configPath,
				}),
				[]string{"run"},
				run,
				func() {})

			registers, _ := fakeCore.Counts()
			if test.coreAgentIPC {
				require.NotZero(t, registers, "consumer never registered, so this test no longer covers configstream")
			} else {
				require.Zero(t, registers, "consumer registered with the core agent although core agent IPC is disabled")
			}
		})
	}
}
