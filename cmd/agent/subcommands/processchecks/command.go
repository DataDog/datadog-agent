// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processchecks implements 'agent processchecks'.
package processchecks

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	processCommand "github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	dualTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-dual"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx"
	wlmcatalogcore "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/defaults"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	remotetraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-remote"
	processComponent "github.com/DataDog/datadog-agent/comp/process"
	rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"
	check "github.com/DataDog/datadog-agent/pkg/cli/subcommands/processchecks"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
)

// getCoreAgentFxOptions returns the fx bundle specific to the core agent that provides the necessary dependencies for the subcommand.
// As the core and process agent have some different dependencies, this function allows having a single subcommand for both agents.
// This function contains all dependencies instead of just those that differ between agents to easily maintain initialization order.
func getCoreAgentFxOptions(cliParams *check.CliParams, bundleParams core.BundleParams) []fx.Option {
	return []fx.Option{
		fx.Supply(cliParams, bundleParams),
		core.Bundle(true),
		hostnameimpl.Module(),

		// Provide eventplatformimpl module
		eventplatformreceiverimpl.Module(),
		eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
		// Provide rdnsquerier module
		rdnsquerierfx.Module(),
		remotetraceroute.Module(),
		// Provide npcollector module
		npcollectorimpl.Module(),
		// Provide the corresponding workloadmeta Params to configure the catalog
		wlmcatalogcore.GetCatalog(),
		// Provide workloadmeta module
		workloadmetafx.Module(defaults.DefaultParams()),
		// Tagger must be initialized after agent config has been setup
		dualTaggerfx.Module(common.DualTaggerParams()),
		workloadfilterfx.Module(),
		processComponent.Bundle(),
		// InitSharedContainerProvider must be called before the application starts so the workloadmeta collector can be initiailized correctly.
		// Since the tagger depends on the workloadmeta collector, we can not make the tagger a dependency of workloadmeta as it would create a circular dependency.
		// TODO: (component) - once we remove the dependency of workloadmeta component from the tagger component
		// we can include the tagger as part of the workloadmeta component.
		fx.Invoke(func(wmeta workloadmeta.Component, tagger tagger.Component, filterStore workloadfilter.Component) {
			proccontainers.InitSharedContainerProvider(wmeta, tagger, filterStore)
		}),
		fx.Provide(func() statsd.ClientInterface {
			return &statsd.NoOpClient{}
		}),
		ipcfx.ModuleReadOnly(),
	}
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	processCommand.OneShotLogParams = log.ForOneShot(string(command.LoggerName), "info", true)
	checkAllowlist := []string{"process", "rtprocess", "container", "rtcontainer", "process_discovery"}
	cmd := check.MakeCommand(func() *processCommand.GlobalParams {
		return &processCommand.GlobalParams{
			ConfFilePath:         globalParams.ConfFilePath,
			ExtraConfFilePath:    globalParams.ExtraConfFilePath,
			SysProbeConfFilePath: globalParams.SysProbeConfFilePath,
			FleetPoliciesDirPath: globalParams.FleetPoliciesDirPath,
		}
	},
		"processchecks",
		checkAllowlist,
		getCoreAgentFxOptions,
	)
	return []*cobra.Command{cmd}
}
