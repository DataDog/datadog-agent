// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package run is the run host-profiler subcommand
package run

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/host-profiler/globalparams"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	configsync "github.com/DataDog/datadog-agent/comp/core/configsync/def"
	configsyncfx "github.com/DataDog/datadog-agent/comp/core/configsync/fx"
	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTaggerFx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote"
	statsd "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
	statsdotel "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/otel"
	hostprofiler "github.com/DataDog/datadog-agent/comp/host-profiler"
	collector "github.com/DataDog/datadog-agent/comp/host-profiler/collector/def"
	collectorimpl "github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	traceagentfx "github.com/DataDog/datadog-agent/comp/trace/agent/fx"
	traceagentcomp "github.com/DataDog/datadog-agent/comp/trace/agent/impl"
	gzipfx "github.com/DataDog/datadog-agent/comp/trace/compression/fx-gzip"
	traceconfigdef "github.com/DataDog/datadog-agent/comp/trace/config/def"
	traceconfigfx "github.com/DataDog/datadog-agent/comp/trace/config/fx"
	payloadmodifierfx "github.com/DataDog/datadog-agent/comp/trace/payload-modifier/fx"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
)

type cliParams struct {
	*globalparams.GlobalParams
	GoRuntimeMetrics bool
}

// MakeCommand creates the `run` command
func MakeCommand(globalConfGetter func() *globalparams.GlobalParams) []*cobra.Command {
	params := &cliParams{}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start Host Profiler",
		Long:  `Runs the Host Profiler to collect host profiling data and send it to Datadog.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			params.GlobalParams = globalConfGetter()
			return runHostProfilerCommand(context.Background(), params)
		},
	}

	cmd.Flags().BoolVar(&params.GoRuntimeMetrics, "go-runtime-metrics", false, "Enable Go runtime metrics collection.")
	return []*cobra.Command{cmd}
}

func validateFlags(params *globalparams.GlobalParams) error {
	// Require at least one configuration source
	if params.ConfFilePath == "" && params.CoreConfPath == "" {
		return errors.New("must provide either --config or --core-config configuration")
	}

	return nil
}

func runHostProfilerCommand(ctx context.Context, cliParams *cliParams) error {
	// Validate flag usage
	if err := validateFlags(cliParams.GlobalParams); err != nil {
		return err
	}

	var opts = []fx.Option{
		hostprofiler.Bundle(collectorimpl.NewParams(cliParams.GlobalParams.ConfigURI(), cliParams.GoRuntimeMetrics)),
		logging.DefaultFxLoggingOption(),
	}

	if cliParams.GlobalParams.CoreConfPath != "" {
		warnBothConfigs := cliParams.GlobalParams.ConfFilePath != ""
		opts = append(opts,
			core.Bundle(),
			remotehostnameimpl.Module(),
			fx.Supply(core.BundleParams{
				ConfigParams: config.NewAgentParams(cliParams.GlobalParams.CoreConfPath),
				LogParams:    log.ForDaemon(command.LoggerName, "log_file", defaultpaths.GetDefaultHostProfilerLogFile()),
			}),
			fx.Provide(collectorimpl.NewExtraFactoriesWithAgentCore),
			fx.Invoke(func(l log.Component) {
				if warnBothConfigs {
					l.Warn("Both OTel and Core Agent configuration paths were provided. The OTel configuration will be ignored and the Core Agent configuration will be used.")
				}
			}),
		)
		opts = append(opts, getRemoteTaggerOptions()...)
		opts = append(opts, getTraceAgentOptions(ctx)...)
		opts = append(opts, getConfigOptions(cliParams.GlobalParams)...)
	} else {
		opts = append(opts,
			fx.Invoke(initStandaloneConfig),
			fx.Provide(collectorimpl.NewExtraFactoriesWithoutAgentCore),
		)
	}

	return fxutil.OneShot(run, opts...)
}

func run(collector collector.Component) error {
	return collector.Run()
}

// initStandaloneConfig performs one-time config setup for standalone mode (no core agent).
// K8S_NODE_IP is set by upstream Helm charts for the node IP; we use it as
// kubernetes_kubelet_host so the kubelet client can resolve the node hostname.
func initStandaloneConfig() {
	const kubeletHostAgentConfig = "kubernetes_kubelet_host"
	pkgconfigenv.DetectFeatures(setup.Datadog())
	k8sNodeIP := os.Getenv("K8S_NODE_IP")
	// If not set, let's keep DD_KUBERNETES_KUBELET_HOST as fallback
	if k8sNodeIP != "" {
		setup.Datadog().Set(kubeletHostAgentConfig, k8sNodeIP, pkgconfigmodel.SourceAgentRuntime)
	} else if _, exists := os.LookupEnv("DD_KUBERNETES_KUBELET_HOST"); exists {
		slog.Warn("DD_KUBERNETES_KUBELET_HOST used as fallback to K8S_NODE_IP but is not officially supported")
	}
}

func getRemoteTaggerOptions() []fx.Option {
	return []fx.Option{
		ipcfx.ModuleReadWrite(),
		remoteTaggerFx.Module(tagger.NewRemoteParams()),
	}
}

func getConfigOptions(params *globalparams.GlobalParams) []fx.Option {
	return []fx.Option{
		configsyncfx.Module(configsync.NewParams(params.SyncTimeout, true, params.SyncOnInitTimeout)),
	}
}

func getTraceAgentOptions(ctx context.Context) []fx.Option {
	return []fx.Option{
		traceagentfx.Module(),
		traceconfigfx.Module(),

		fx.Supply(&traceagentcomp.Params{
			CPUProfile:               "",
			MemProfile:               "",
			PIDFilePath:              "",
			DisableInternalProfiling: true,
		}),
		fx.Provide(func(cfg traceconfigdef.Component) telemetry.TelemetryCollector {
			return telemetry.NewCollector(cfg.Object())
		}),
		fx.Supply(metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{})),
		fx.Provide(func(client *metricsclient.StatsdClientWrapper) statsd.Component {
			return statsdotel.NewOTelStatsd(client)
		}),

		gzipfx.Module(),
		payloadmodifierfx.NilModule(),
		fx.Supply(fx.Annotate(ctx, fx.As(new(context.Context)))),

		fx.Decorate(func(config config.Component) config.Component {
			config.Set("apm_config.debug.port", 0, pkgconfigmodel.SourceDefault)           // Disabled as in the otel-agent
			config.Set(setup.OTLPTracePort, 0, pkgconfigmodel.SourceDefault)               // Disabled as in the otel-agent
			config.Set("apm_config.receiver_enabled", false, pkgconfigmodel.SourceDefault) // disable HTTP receiver
			return config
		}),
	}
}
