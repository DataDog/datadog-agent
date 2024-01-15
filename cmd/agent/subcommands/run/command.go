// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run implements 'agent run' (and deprecated 'agent start').
package run

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/agents/core/coreimpl"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/defaults"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	langDetectionClimpl "github.com/DataDog/datadog-agent/comp/languagedetection/client/clientimpl"
	"github.com/DataDog/datadog-agent/comp/logs"
	"github.com/DataDog/datadog-agent/comp/metadata"
	"github.com/DataDog/datadog-agent/comp/ndmtmp"
	"github.com/DataDog/datadog-agent/comp/netflow"
	"github.com/DataDog/datadog-agent/comp/otelcol"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/collector"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &coreimpl.CliParams{
		GlobalParams: globalParams,
	}
	runE := func(*cobra.Command, []string) error {
		// TODO: once the agent is represented as a component, and not a function (run),
		// this will use `fxutil.Run` instead of `fxutil.OneShot`.
		return fxutil.OneShot(coreimpl.Run,
			fx.Supply(cliParams),
			fx.Supply(core.BundleParams{
				ConfigParams:         config.NewAgentParams(globalParams.ConfFilePath),
				SecretParams:         secrets.NewEnabledParams(),
				SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath)),
				LogParams:            logimpl.ForDaemon(command.LoggerName, "log_file", path.DefaultLogFile),
			}),
			getSharedFxOption(),
			getPlatformModules(),
		)
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Agent",
		Long:  `Runs the agent in the foreground`,
		RunE:  runE,
	}
	runCmd.Flags().StringVarP(&cliParams.PidfilePath, "pidfile", "p", "", "path to the pidfile")

	startCmd := &cobra.Command{
		Use:        "start",
		Deprecated: "Use \"run\" instead to start the Agent",
		RunE:       runE,
	}
	startCmd.Flags().StringVarP(&cliParams.PidfilePath, "pidfile", "p", "", "path to the pidfile")

	return []*cobra.Command{startCmd, runCmd}
}

func getSharedFxOption() fx.Option {
	return fx.Options(
		fx.Supply(flare.NewParams(
			path.GetDistPath(),
			path.PyChecksPath,
			path.DefaultLogFile,
			path.DefaultJmxLogFile,
			path.DefaultDogstatsDLogFile,
		)),
		flare.Module(),
		core.Bundle(),
		fx.Supply(dogstatsdServer.Params{
			Serverless: false,
		}),
		forwarder.Bundle(),
		fx.Provide(func(config config.Component, log log.Component) defaultforwarder.Params {
			params := defaultforwarder.NewParams(config, log)
			// Enable core agent specific features like persistence-to-disk
			params.Options.EnabledFeatures = defaultforwarder.SetFeature(params.Options.EnabledFeatures, defaultforwarder.CoreFeatures)
			return params
		}),

		// workloadmeta setup
		collectors.GetCatalog(),
		fx.Provide(defaults.DefaultParams),
		workloadmeta.Module(),
		apiimpl.Module(),

		dogstatsd.Bundle(),
		otelcol.Bundle(),
		rcclient.Module(),

		// TODO: (components) - some parts of the agent (such as the logs agent) implicitly depend on the global state
		// set up by LoadComponents. In order for components to use lifecycle hooks that also depend on this global state, we
		// have to ensure this code gets run first. Once the common package is made into a component, this can be removed.
		//
		// Workloadmeta component needs to be initialized before this hook is executed, and thus is included
		// in the function args to order the execution. This pattern might be worth revising because it is
		// error prone.
		fx.Invoke(func(lc fx.Lifecycle, demultiplexer demultiplexer.Component, _ workloadmeta.Component, secretResolver secrets.Component) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {

					// create and setup the Autoconfig instance
					common.LoadComponents(demultiplexer, secretResolver, pkgconfig.Datadog.GetString("confd_path"))
					return nil
				},
			})
		}),
		logs.Bundle(),
		langDetectionClimpl.Module(),
		metadata.Bundle(),
		// injecting the aggregator demultiplexer to FX until we migrate it to a proper component. This allows
		// other already migrated components to request it.
		fx.Provide(func(config config.Component) demultiplexerimpl.Params {
			params := demultiplexerimpl.NewDefaultParams()
			params.EnableNoAggregationPipeline = config.GetBool("dogstatsd_no_aggregation_pipeline")
			return params
		}),
		demultiplexerimpl.Module(),
		orchestratorForwarderImpl.Module(),
		fx.Supply(orchestratorForwarderImpl.NewDefaultParams()),
		// injecting the shared Serializer to FX until we migrate it to a prpoper component. This allows other
		// already migrated components to request it.
		fx.Provide(func(demuxInstance demultiplexer.Component) serializer.MetricSerializer {
			return demuxInstance.Serializer()
		}),
		ndmtmp.Bundle(),
		netflow.Bundle(),
		fx.Provide(func(demultiplexer demultiplexer.Component) optional.Option[collector.Collector] {
			return optional.NewOption(common.LoadCollector(demultiplexer))
		}),
	)
}
