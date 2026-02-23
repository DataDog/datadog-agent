// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package run is the run host-profiler subcommand
package run

import (
	"context"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/host-profiler/globalparams"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTaggerFx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	statsdotel "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/otel"
	hostprofiler "github.com/DataDog/datadog-agent/comp/host-profiler"
	collector "github.com/DataDog/datadog-agent/comp/host-profiler/collector/def"
	collectorimpl "github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	traceagentfx "github.com/DataDog/datadog-agent/comp/trace/agent/fx"
	traceagentcomp "github.com/DataDog/datadog-agent/comp/trace/agent/impl"
	gzipfx "github.com/DataDog/datadog-agent/comp/trace/compression/fx-gzip"
	traceconfig "github.com/DataDog/datadog-agent/comp/trace/config"
	payloadmodifierfx "github.com/DataDog/datadog-agent/comp/trace/payload-modifier/fx"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
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

func runHostProfilerCommand(ctx context.Context, cliParams *cliParams) error {
	var opts = []fx.Option{
		hostprofiler.Bundle(collectorimpl.NewParams(cliParams.GlobalParams.ConfFilePath, cliParams.GoRuntimeMetrics)),
		logging.DefaultFxLoggingOption(),
	}

	if cliParams.GlobalParams.CoreConfPath != "" {
		opts = append(opts,
			core.Bundle(true),
			remotehostnameimpl.Module(),
			fx.Supply(core.BundleParams{
				ConfigParams: config.NewAgentParams(cliParams.GlobalParams.CoreConfPath),
				LogParams:    log.ForDaemon(command.LoggerName, "log_file", setup.DefaultHostProfilerLogFile),
			}),
			fx.Provide(collectorimpl.NewExtraFactoriesWithAgentCore),
		)
		opts = append(opts, getRemoteTaggerOptions()...)
		opts = append(opts, getTraceAgentOptions(ctx)...)

	} else {
		opts = append(opts, fx.Provide(collectorimpl.NewExtraFactoriesWithoutAgentCore))
	}

	return fxutil.OneShot(run, opts...)
}

func run(collector collector.Component) error {
	return collector.Run()
}

func getRemoteTaggerOptions() []fx.Option {
	return []fx.Option{
		ipcfx.ModuleReadOnly(),
		remoteTaggerFx.Module(tagger.NewRemoteParams()),
	}
}

func getTraceAgentOptions(ctx context.Context) []fx.Option {
	return []fx.Option{
		traceagentfx.Module(),
		traceconfig.Module(),

		fx.Supply(&traceagentcomp.Params{
			CPUProfile:               "",
			MemProfile:               "",
			PIDFilePath:              "",
			DisableInternalProfiling: true,
		}),
		fx.Provide(func(cfg traceconfig.Component) telemetry.TelemetryCollector {
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
