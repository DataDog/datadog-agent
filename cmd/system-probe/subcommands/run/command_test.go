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

func TestRunCommand(t *testing.T) {
	// Because fx.Invoke builds the ipc component, we need to ensure we
	// have a valid auth token before building the app for real.
	testDir := t.TempDir()

	// configstream is on by default, so the fx graph blocks until it seeds from a core agent.
	configstreambootstrap.UseDynamicSchema(t)
	fakeCore := configstreamtestutil.StartFakeCoreAgent(t, testDir)

	configPath := filepath.Join(testDir, "datadog.yaml")
	config := "hostname: test\n" + fakeCore.ConfigYAML()
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0644))

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
	require.NotZero(t, registers, "consumer never registered, so this test no longer covers configstream")
}
