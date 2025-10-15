// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package check

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote"
	wmcatalogremote "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-remote"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	processComponent "github.com/DataDog/datadog-agent/comp/process"
	rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/processchecks"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
)

// getProcessAgentFxOptions returns the fx bundle specific to the process agent that provides the necessary dependencies for the subcommand.
// As the core and process agent have some different dependencies, this function allows having a single subcommand for both agents.
// This function contains all dependencies instead of just those that differ between agents to easily maintain initialization order.
func getProcessAgentFxOptions(cliParams *processchecks.CliParams, bundleParams core.BundleParams) []fx.Option {
	return []fx.Option{
		fx.Supply(cliParams, bundleParams),
		core.Bundle(),
		secretsfx.Module(),

		// Provide eventplatformimpl module
		eventplatformreceiverimpl.Module(),
		eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
		// Provide rdnsquerier module
		rdnsquerierfx.Module(),
		// Provide npcollector module
		npcollectorimpl.Module(),
		// Provide the corresponding workloadmeta Params to configure the catalog
		wmcatalogremote.GetCatalog(),
		// Provide workloadmeta module
		workloadmetafx.Module(workloadmeta.Params{
			AgentType: workloadmeta.Remote,
		}),

		// Tagger must be initialized after agent config has been setup
		remoteTaggerfx.Module(tagger.NewRemoteParams()),
		processComponent.Bundle(),
		// InitSharedContainerProvider must be called before the application starts so the workloadmeta collector can be initiailized correctly.
		// Since the tagger depends on the workloadmeta collector, we can not make the tagger a dependency of workloadmeta as it would create a circular dependency.
		// TODO: (component) - once we remove the dependency of workloadmeta component from the tagger component
		// we can include the tagger as part of the workloadmeta component.
		fx.Invoke(func(wmeta workloadmeta.Component, tagger tagger.Component) {
			proccontainers.InitSharedContainerProvider(wmeta, tagger)
		}),
		fx.Provide(func() statsd.ClientInterface {
			return &statsd.NoOpClient{}
		}),
		ipcfx.ModuleReadOnly(),
	}
}

// Commands returns a slice of subcommands for the `check` command in the Process Agent
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	checkAllowlist := []string{"process", "rtprocess", "container", "rtcontainer", "connections", "process_discovery", "process_events"}
	return []*cobra.Command{processchecks.MakeCommand(func() *command.GlobalParams {
		return &command.GlobalParams{
			ConfFilePath:         globalParams.ConfFilePath,
			ExtraConfFilePath:    globalParams.ExtraConfFilePath,
			SysProbeConfFilePath: globalParams.SysProbeConfFilePath,
			FleetPoliciesDirPath: globalParams.FleetPoliciesDirPath,
		}
	}, "check", checkAllowlist, getProcessAgentFxOptions)}
}
