// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processchecks

import (
	"os"
	"path"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	processComponent "github.com/DataDog/datadog-agent/comp/process"
	rdnsquerierfxmock "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx-mock"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	// Because we use fx.Invoke some components are built
	// we need to ensure we have a valid auth token
	testDir := t.TempDir()

	configPath := path.Join(testDir, "datadog.yaml")
	err := os.WriteFile(configPath, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	configComponent.NewMockFromYAMLFile(t, configPath)

	// Check command should work when an Agent is running, so we need to
	// ensure we have existing IPC auth artifacts.
	// This is done by building the IPC component
	// with the `ipcfx.ModuleReadWrite()` module.
	fxutil.Test[ipc.Component](t,
		ipcfx.ModuleReadWrite(),
		core.MockBundle(),
		hostnameimpl.MockModule(),
	)

	// closely mirrors what the agents would use, but with mock modules where possible
	getTestFxOptions := func(cliParams *CliParams, bundleParams core.BundleParams) []fx.Option {
		return []fx.Option{
			fx.Supply(cliParams, bundleParams),
			core.Bundle(),
			hostnameimpl.Module(),
			secretsfx.Module(),

			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			workloadfilterfxmock.MockModule(),
			fx.Provide(func() tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
			rdnsquerierfxmock.MockModule(),
			npcollectorimpl.MockModule(),
			processComponent.Bundle(),

			// InitSharedContainerProvider must be called before the application starts so the workloadmeta collector can be initialized correctly.
			fx.Invoke(func(wmeta workloadmeta.Component, tagger tagger.Component, filterStore workloadfilter.Component) {
				proccontainers.InitSharedContainerProvider(wmeta, tagger, filterStore)
			}),
			fx.Provide(func() statsd.ClientInterface {
				return &statsd.NoOpClient{}
			}),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
		}
	}

	commands := []*cobra.Command{
		MakeCommand(
			func() *command.GlobalParams {
				return &command.GlobalParams{
					ConfFilePath: configPath,
				}
			},
			"processchecks", []string{"process"},
			getTestFxOptions),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"processchecks", "process"},
		RunCheckCmd,
		func(_ *CliParams) {},
	)
}
