// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run implements the run subcommand for the 'trace-agent' command.
package run

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	yaml "go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	autoexit "github.com/DataDog/datadog-agent/comp/agent/autoexit/def"
	autoexitfx "github.com/DataDog/datadog-agent/comp/agent/autoexit/fx"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	agenttelemetryfx "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
	configstreamconsumerfx "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/fx"
	"github.com/DataDog/datadog-agent/comp/core/configsync/configsyncimpl"
	delegatedauthfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx"
	fxinstrumentation "github.com/DataDog/datadog-agent/comp/core/fxinstrumentation/fx"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logtracefx "github.com/DataDog/datadog-agent/comp/core/log/fx-trace"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	remoteagentfx "github.com/DataDog/datadog-agent/comp/core/remoteagent/fx-trace"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	optionalRemoteTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-optional-remote"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryfx "github.com/DataDog/datadog-agent/comp/core/telemetry/fx"
	statsdFx "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/fx"
	"github.com/DataDog/datadog-agent/comp/trace"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	traceagentimpl "github.com/DataDog/datadog-agent/comp/trace/agent/impl"
	zstdfx "github.com/DataDog/datadog-agent/comp/trace/compression/fx-zstd"
	traceconfigdef "github.com/DataDog/datadog-agent/comp/trace/config/def"
	traceconfigimpl "github.com/DataDog/datadog-agent/comp/trace/config/impl"
	payloadmodifierfx "github.com/DataDog/datadog-agent/comp/trace/payload-modifier/fx"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// configstreamConsumerEnabledEnvVar is the environment variable that controls whether the configstream consumer is enabled.
const configstreamConsumerEnabledEnvVar = "DD_REMOTE_AGENT_CONFIGSTREAM_CONSUMER_ENABLED"

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
	opts := []fx.Option{
		// ctx is required to be supplied from here, as Windows needs to inject its own context
		// to allow the agent to work as a service.
		fx.Provide(func() context.Context { return ctx }), // fx.Supply(ctx) fails with a missing type error.
		fx.Supply(coreconfig.NewAgentParams(cliParams.ConfPath, coreconfig.WithFleetPoliciesDirPath(cliParams.FleetPoliciesDirPath))),
		secretsfx.Module(),
		delegatedauthfx.Module(),
		telemetryfx.Module(),
		coreconfig.Module(),
		fx.Provide(func() log.Params {
			return log.ForDaemon("TRACE", "apm_config.log_file", traceconfigimpl.DefaultLogFilePath)
		}),
		logtracefx.Module(),
		autoexitfx.Module(),
		statsdFx.Module(),
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
		fx.Invoke(func(_ traceconfigdef.Component) {}),
		// Required to avoid cyclic imports.
		fx.Provide(func(cfg traceconfigdef.Component) telemetry.TelemetryCollector {
			return telemetry.NewCollector(cfg.Object())
		}),
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
		agenttelemetryfx.Module(),
	}
	// Wire the consumer before trace-agent so its OnStart blocks on snapshot first.
	if isConfigstreamEnabled(cliParams.ConfPath) {
		opts = append(opts, configstreamFxOptions())
	}
	opts = append(opts,
		// Force the instantiation of the components
		fx.Invoke(func(_ traceagent.Component, _ autoexit.Component) {}),
		fx.Invoke(func(tm coretelemetry.Component) {
			api.InitTelemetry(tm)
		}),
		fx.Invoke(func(_ option.Option[agenttelemetry.Component]) {}),
	)
	err := fxutil.Run(opts...)
	if err != nil && errors.Is(err, traceagentimpl.ErrAgentDisabled) {
		return nil
	}
	return err
}

// configstreamFxOptions returns FX options for the config stream consumer.
// Only include this when remote_agent.configstream.consumer.enabled is true.
func configstreamFxOptions() fx.Option {
	return fx.Options(
		// Expose config.Component as model.Writer for the config stream consumer to write remote config into.
		fx.Provide(func(c coreconfig.Component) model.Writer {
			return c
		}),
		// SessionIDProvider from RAR: the remote agent component implements this when registry is enabled.
		fx.Provide(func(ra remoteagent.Component) configstreamconsumer.SessionIDProvider {
			if ra == nil {
				return nil
			}
			if p, ok := ra.(configstreamconsumer.SessionIDProvider); ok {
				return p
			}
			return nil
		}),
		fx.Provide(func(c coreconfig.Component, sessionProvider configstreamconsumer.SessionIDProvider) configstreamconsumer.Params {
			host := c.GetString("cmd_host")
			port := c.GetInt("cmd_port")
			if port <= 0 {
				port = 5001
			}
			return configstreamconsumer.Params{
				ClientName:        "trace-agent",
				CoreAgentAddress:  net.JoinHostPort(host, strconv.Itoa(port)),
				SessionIDProvider: sessionProvider,
			}
		}),
		configstreamconsumerfx.Module(),
		// Trigger instantiation; OnStart handles the blocking wait internally.
		fx.Invoke(func(_ configstreamconsumer.Component) {}),
	)
}

// isConfigstreamEnabled is a pre-FX feature flag check; the env var takes precedence over YAML.
func isConfigstreamEnabled(cliConfigPath string) bool {
	if v, ok := os.LookupEnv(configstreamConsumerEnabledEnvVar); ok {
		if enabled, err := strconv.ParseBool(v); err == nil {
			return enabled
		}
	}
	for _, path := range []string{cliConfigPath, coreconfig.DefaultConfPath} {
		if path == "" {
			continue
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			path = filepath.Join(path, "datadog.yaml")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg struct {
			RemoteAgent struct {
				ConfigStream struct {
					Consumer struct {
						Enabled bool `yaml:"enabled"`
					} `yaml:"consumer"`
				} `yaml:"configstream"`
			} `yaml:"remote_agent"`
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return false
		}
		return cfg.RemoteAgent.ConfigStream.Consumer.Enabled
	}
	return false
}
