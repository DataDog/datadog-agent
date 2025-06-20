// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunCheckCmdCommand(t *testing.T) {
	// Because we uses fx.Invoke some components are built
	// we need to ensure we have a valid auth token
	testDir := t.TempDir()

	configPath := path.Join(testDir, "datadog.yaml")
	err := os.WriteFile(configPath, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	configComponent.NewMockFromYAMLFile(t, configPath)

	// Check command should work when an Agent is running, so we need to
	// ensure we have exisiting IPC auth artifacts.
	// This is done by building the IPC component
	// with the `ipcfx.ModuleReadWrite()` module.
	fxutil.Test[ipc.Component](t,
		ipcfx.ModuleReadWrite(),
		core.MockBundle(),
	)

	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{
			ConfFilePath: configPath,
		}),
		[]string{"check", "process"},
		RunCheckCmd,
		func(_ *CliParams) {},
	)
}
