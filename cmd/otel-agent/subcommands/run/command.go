// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package run

import (
	"context"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/confmap"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	agentConfig "github.com/DataDog/datadog-agent/cmd/otel-agent/config"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync"
	"github.com/DataDog/datadog-agent/comp/core/configsync/configsyncimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logtracefx "github.com/DataDog/datadog-agent/comp/core/log/fx-trace"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	collectorcontribFx "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/fx"
	collectordef "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	collectorfx "github.com/DataDog/datadog-agent/comp/otelcol/collector/fx"
	converter "github.com/DataDog/datadog-agent/comp/otelcol/converter/def"
	converterfx "github.com/DataDog/datadog-agent/comp/otelcol/converter/fx"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/logsagentpipelineimpl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl/strategy"
	traceagentfx "github.com/DataDog/datadog-agent/comp/trace/agent/fx"
	traceagentcomp "github.com/DataDog/datadog-agent/comp/trace/agent/impl"
	gzipfx "github.com/DataDog/datadog-agent/comp/trace/compression/fx-gzip"
	traceconfig "github.com/DataDog/datadog-agent/comp/trace/config"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"go.uber.org/fx"
)

// MakeCommand creates the `run` command
func MakeCommand(globalConfGetter func() *subcommands.GlobalParams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Starting OpenTelemetry Collector",
		RunE: func(_ *cobra.Command, _ []string) error {
			globalParams := globalConfGetter()
			return runOTelAgentCommand(context.Background(), globalParams)
		},
	}
	return cmd
}

type orchestratorinterfaceimpl struct {
	f defaultforwarder.Forwarder
}

func newOrchestratorinterfaceimpl(f defaultforwarder.Forwarder) orchestratorinterface.Component {
	return &orchestratorinterfaceimpl{
		f: f,
	}
}

func (o *orchestratorinterfaceimpl) Get() (defaultforwarder.Forwarder, bool) {
	return o.f, true
}

func (o *orchestratorinterfaceimpl) Reset() {
	o.f = nil
}

func runOTelAgentCommand(ctx context.Context, params *subcommands.GlobalParams, opts ...fx.Option) error {
	acfg, err := agentConfig.NewConfigComponent(context.Background(), params.CoreConfPath, params.ConfPaths)
	if err != nil && err != agentConfig.ErrNoDDExporter {
		return err
	}
	uris := append(params.ConfPaths, params.Sets...)
	if err == agentConfig.ErrNoDDExporter {
		return fxutil.Run(
			fx.Supply(uris),
			fx.Provide(func() coreconfig.Component {
				return acfg
			}),
			converterfx.Module(),
			fx.Provide(func(cp converter.Component) confmap.Converter {
				return cp
			}),
			collectorcontribFx.Module(),
			collectorfx.ModuleNoAgent(),
			fx.Options(opts...),
			fx.Invoke(func(_ collectordef.Component) {
			}),
		)
	}

	return fxutil.Run(
		ForwarderBundle(),
		logtracefx.Module(),
		inventoryagentimpl.Module(),
		fx.Supply(metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{})),
		fx.Provide(func(client *metricsclient.StatsdClientWrapper) statsd.Component {
			return statsd.NewOTelStatsd(client)
		}),
		fetchonlyimpl.Module(),
		collectorfx.Module(),
		collectorcontribFx.Module(),
		converterfx.Module(),
		fx.Provide(func(cp converter.Component) confmap.Converter {
			return cp
		}),
		fx.Provide(func() (coreconfig.Component, error) {
			pkgconfigenv.DetectFeatures(acfg)
			return acfg, nil
		}),
		fxutil.ProvideOptional[coreconfig.Component](),
		workloadmetafx.Module(workloadmeta.Params{
			AgentType:  workloadmeta.NodeAgent,
			InitHelper: common.GetWorkloadmetaInit(),
		}),
		fx.Supply(uris),
		fx.Provide(func(h hostnameinterface.Component) (serializerexporter.SourceProviderFunc, error) {
			return h.Get, nil
		}),
		remotehostnameimpl.Module(),

		fx.Provide(func(_ coreconfig.Component) log.Params {
			return log.ForDaemon(params.LoggerName, "log_file", pkgconfigsetup.DefaultOTelAgentLogFile)
		}),
		logsagentpipelineimpl.Module(),
		// We create strategy.ZlibStrategy directly to avoid build tags
		fx.Provide(strategy.NewZlibStrategy),
		fx.Provide(func(s *strategy.ZlibStrategy) compression.Component {
			return s
		}),
		fx.Provide(serializer.NewSerializer),
		// For FX to provide the serializer.MetricSerializer from the serializer.Serializer
		fx.Provide(func(s *serializer.Serializer) serializer.MetricSerializer {
			return s
		}),
		fx.Provide(func(h serializerexporter.SourceProviderFunc, l log.Component) (string, error) {
			hn, err := h(context.Background())
			if err != nil {
				return "", err
			}
			l.Info("Using ", "hostname", hn)

			return hn, nil
		}),

		fx.Provide(func(c defaultforwarder.Component) (defaultforwarder.Forwarder, error) {
			return defaultforwarder.Forwarder(c), nil
		}),
		fx.Provide(newOrchestratorinterfaceimpl),
		fx.Options(opts...),
		fx.Invoke(func(_ collectordef.Component, _ defaultforwarder.Forwarder, _ optional.Option[logsagentpipeline.Component]) {
		}),

		// TODO: don't rely on this pattern; remove this `OptionalModuleWithParams` thing
		//       and instead adapt OptionalModule to allow parameter passing naturally.
		//       See: https://github.com/DataDog/datadog-agent/pull/28386
		configsyncimpl.OptionalModuleWithParams(),
		fx.Provide(func() configsyncimpl.Params {
			return configsyncimpl.NewParams(params.SyncTimeout, params.SyncDelay, true)
		}),

		fx.Provide(func() tagger.Params {
			return tagger.NewNodeRemoteTaggerParamsWithFallback()
		}),
		taggerimpl.Module(),
		telemetryimpl.Module(),
		fx.Provide(func(cfg traceconfig.Component) telemetry.TelemetryCollector {
			return telemetry.NewCollector(cfg.Object())
		}),
		gzipfx.Module(),

		// ctx is required to be supplied from here, as Windows needs to inject its own context
		// to allow the agent to work as a service.
		fx.Provide(func() context.Context { return ctx }), // fx.Supply(ctx) fails with a missing type error.

		// TODO: consider adding configsync.Component as an explicit dependency for traceconfig
		//       to avoid this sort of dependency tree hack.
		fx.Provide(func(deps traceconfig.Dependencies, _ optional.Option[configsync.Component]) (traceconfig.Component, error) {
			// TODO: this would be much better if we could leverage traceconfig.Module
			//       Must add a new parameter to traconfig.Module to handle this.
			return traceconfig.NewConfig(deps)
		}),
		fx.Supply(traceconfig.Params{FailIfAPIKeyMissing: false}),

		fx.Supply(&traceagentcomp.Params{
			CPUProfile:  "",
			MemProfile:  "",
			PIDFilePath: "",
		}),
		traceagentfx.Module(),
	)
}

// ForwarderBundle returns the fx.Option for the forwarder bundle.
// TODO: cleanup the forwarder instantiation with fx.
// This is a bit of a hack because we need to enforce optional.Option[configsync.Component]
// is passed to newForwarder to enforce the correct instantiation order. Currently, the
// new forwarder.BundleWithProvider makes a few assumptions in its generic prototype, and
// this is the current workaround to leverage it.
func ForwarderBundle() fx.Option {
	return defaultforwarder.ModulWithOptionTMP(
		fx.Provide(func(_ optional.Option[configsync.Component]) defaultforwarder.Params {
			return defaultforwarder.NewParams()
		}))
}
