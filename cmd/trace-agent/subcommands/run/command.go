// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run implements the run subcommand for the 'trace-agent' command.
package run

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit/autoexitimpl"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	agenttelemetryfx "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync/configsyncimpl"
	fxinstrumentation "github.com/DataDog/datadog-agent/comp/core/fxinstrumentation/fx"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logtracefx "github.com/DataDog/datadog-agent/comp/core/log/fx-trace"
	remoteagentfx "github.com/DataDog/datadog-agent/comp/core/remoteagent/fx-trace"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	optionalRemoteTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-optional-remote"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/comp/trace"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	traceagentimpl "github.com/DataDog/datadog-agent/comp/trace/agent/impl"
	zstdfx "github.com/DataDog/datadog-agent/comp/trace/compression/fx-zstd"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	payloadmodifierfx "github.com/DataDog/datadog-agent/comp/trace/payload-modifier/fx"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// MakeCommand returns the run subcommand for the 'trace-agent' command.
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {
	cliParams := &Params{}
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

func setParamFlags(cmd *cobra.Command, cliParams *Params) {
	cmd.PersistentFlags().StringVarP(&cliParams.PIDFilePath, "pidfile", "p", "", "path for the PID file to be created")
	cmd.PersistentFlags().StringVarP(&cliParams.CPUProfile, "cpu-profile", "l", "",
		"enables CPU profiling and specifies profile path.")
	cmd.PersistentFlags().StringVarP(&cliParams.MemProfile, "mem-profile", "m", "",
		"enables memory profiling and specifies profile.")

	setOSSpecificParamFlags(cmd, cliParams)
}

func runTraceAgentProcess(ctx context.Context, cliParams *Params, defaultConfPath string) error {
	if cliParams.ConfPath == "" {
		cliParams.ConfPath = defaultConfPath
	}
	err := fxutil.Run(
		// ctx is required to be supplied from here, as Windows needs to inject its own context
		// to allow the agent to work as a service.
		fx.Provide(func() context.Context { return ctx }), // fx.Supply(ctx) fails with a missing type error.
		fx.Supply(coreconfig.NewAgentParams(cliParams.ConfPath, coreconfig.WithFleetPoliciesDirPath(cliParams.FleetPoliciesDirPath))),
		secretsfx.Module(),
		telemetryimpl.Module(),
		coreconfig.Module(),
		fx.Provide(func() log.Params {
			return log.ForDaemon("TRACE", "apm_config.log_file", config.DefaultLogFilePath)
		}),
		logtracefx.Module(),
		autoexitimpl.Module(),
		statsd.Module(),
		optionalRemoteTaggerfx.Module(
			tagger.OptionalRemoteParams{
				// We disable the remote tagger *only* if we detect that the
				// trace-agent is running in the Azure App Services (AAS)
				// Extension. The Extension only includes a trace-agent and the
				// dogstatsd binary, and cannot include the core agent. We know
				// that we do not need the container tagging provided by the
				// remote tagger in this environment, so we can use the noop
				// tagger instead.
				Disable: func(_ coreconfig.Component) bool { return serverlessenv.IsAzureAppServicesExtension() },
			},
			tagger.NewRemoteParams()),
		fx.Invoke(func(_ config.Component) {}),
		// Required to avoid cyclic imports.
		fx.Provide(func(cfg config.Component) telemetry.TelemetryCollector { return telemetry.NewCollector(cfg.Object()) }),
		fx.Supply(&traceagentimpl.Params{
			CPUProfile:  cliParams.CPUProfile,
			MemProfile:  cliParams.MemProfile,
			PIDFilePath: cliParams.PIDFilePath,
		}),
		zstdfx.Module(),
		payloadmodifierfx.Module(),
		trace.Bundle(),
		ipcfx.ModuleReadWrite(),
		configsyncimpl.Module(configsyncimpl.NewDefaultParams()),
		fxinstrumentation.Module(),
		remoteagentfx.Module(),
		// Force the instantiation of the components
		fx.Invoke(func(_ traceagent.Component, _ autoexit.Component) {}),
		agenttelemetryfx.Module(),
		fx.Invoke(func(tm coretelemetry.Component) {
			api.InitTelemetry(tm)
		}),
		fx.Invoke(func(_ option.Option[agenttelemetry.Component]) {}),
	)
	if err != nil && errors.Is(err, traceagentimpl.ErrAgentDisabled) {
		return nil
	}
	return err
}
