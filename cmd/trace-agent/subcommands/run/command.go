// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//
//nolint:revive // TODO(APM) Fix revive linter
package run

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	corelogimpl "github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/trace"
	"github.com/DataDog/datadog-agent/comp/trace/agent"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MakeCommand returns the run subcommand for the 'trace-agent' command.
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {

	cliParams := &RunParams{}
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start datadog trace-agent.",
		Long:  `The Datadog trace-agent aggregates, samples, and forwards traces to datadog submitted by tracers loaded into your application.`,
		RunE: func(*cobra.Command, []string) error {
			cliParams.GlobalParams = globalParamsGetter()
			return runTraceAgentCommand(cliParams, cliParams.ConfPath)
		},
	}

	setParamFlags(runCmd, cliParams)

	return runCmd
}

func setParamFlags(cmd *cobra.Command, cliParams *RunParams) {
	cmd.PersistentFlags().StringVarP(&cliParams.PIDFilePath, "pidfile", "p", "", "path for the PID file to be created")
	cmd.PersistentFlags().StringVarP(&cliParams.CPUProfile, "cpu-profile", "l", "",
		"enables CPU profiling and specifies profile path.")
	cmd.PersistentFlags().StringVarP(&cliParams.MemProfile, "mem-profile", "m", "",
		"enables memory profiling and specifies profile.")

	setOSSpecificParamFlags(cmd, cliParams)
}

func runTraceAgentProcess(ctx context.Context, cliParams *RunParams, defaultConfPath string) error {
	if cliParams.ConfPath == "" {
		cliParams.ConfPath = defaultConfPath
	}
	err := fxutil.Run(
		// ctx is required to be supplied from here, as Windows needs to inject its own context
		// to allow the agent to work as a service.
		fx.Provide(func() context.Context { return ctx }), // fx.Supply(ctx) fails with a missing type error.
		fx.Supply(coreconfig.NewAgentParams(cliParams.ConfPath)),
		secretsimpl.Module(),
		fx.Supply(secrets.NewEnabledParams()),
		coreconfig.Module(),
		fx.Provide(func() corelogimpl.Params {
			return corelogimpl.ForDaemon("TRACE", "apm_config.log_file", config.DefaultLogFilePath)
		}),
		corelogimpl.TraceModule(),
		// setup workloadmeta
		collectors.GetCatalog(),
		fx.Supply(workloadmeta.Params{
			AgentType:  workloadmeta.NodeAgent,
			InitHelper: common.GetWorkloadmetaInit(),
		}),
		workloadmeta.Module(),
		statsd.Module(),
		fx.Provide(func(coreConfig coreconfig.Component) tagger.Params {
			if coreConfig.GetBool("apm_config.remote_tagger") {
				return tagger.NewNodeRemoteTaggerParamsWithFallback()
			}
			return tagger.NewTaggerParams()
		}),
		tagger.Module(),
		fx.Invoke(func(_ config.Component) {}),
		// Required to avoid cyclic imports.
		fx.Provide(func(cfg config.Component) telemetry.TelemetryCollector { return telemetry.NewCollector(cfg.Object()) }),
		fx.Supply(&agent.Params{
			CPUProfile:  cliParams.CPUProfile,
			MemProfile:  cliParams.MemProfile,
			PIDFilePath: cliParams.PIDFilePath,
		}),
		trace.Bundle(),
		forwarder.Bundle(),
		eventplatformimpl.Module(),
		eventplatformreceiverimpl.Module(),
		fx.Provide(eventplatformimpl.NewDefaultParams),
		hostnameimpl.Module(),
		fx.Invoke(func(_ agent.Component) {}),
	)
	if err != nil && errors.Is(err, agent.ErrAgentDisabled) {
		return nil
	}
	return err
}
