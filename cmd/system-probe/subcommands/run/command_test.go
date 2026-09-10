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
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunCommand(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{
			name:   "default",
			config: "hostname: test\n",
		},
		{
			// With core agent IPC disabled every component that would stream from
			// the core agent falls back to a local or no-op implementation, and two
			// of the gates hand fx a nil Comp. TestOneShotSubcommand validates the
			// graph and then actually starts it, so this covers the whole fallback
			// assembly rather than the gates individually.
			name: "core agent IPC disabled",
			config: "hostname: test\n" +
				"remote_agent:\n" +
				"  core_agent_ipc:\n" +
				"    enabled: false\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Because fx.Invoke builds the ipc component, we need to ensure we
			// have a valid auth token before building the app for real.
			testDir := t.TempDir()

			configPath := filepath.Join(testDir, "datadog.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(test.config), 0644))

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
		})
	}
}
