// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"go.opentelemetry.io/collector/confmap"

	agentConfig "github.com/DataDog/datadog-agent/cmd/otel-agent/config"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	agenttelemetryfx "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync"
	"github.com/DataDog/datadog-agent/comp/core/configsync/configsyncimpl"
	fxinstrumentation "github.com/DataDog/datadog-agent/comp/core/fxinstrumentation/fx"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	logtracefx "github.com/DataDog/datadog-agent/comp/core/log/fx-trace"
	pid "github.com/DataDog/datadog-agent/comp/core/pid/def"
	pidfx "github.com/DataDog/datadog-agent/comp/core/pid/fx"
	pidimpl "github.com/DataDog/datadog-agent/comp/core/pid/impl"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTaggerFx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-optional-remote"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	workloadmetainit "github.com/DataDog/datadog-agent/comp/core/workloadmeta/init"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	statsdotel "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/otel"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface"
	logconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	collectorcontribFx "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/fx"
	collectordef "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	collectorfx "github.com/DataDog/datadog-agent/comp/otelcol/collector/fx"
	collectorimpl "github.com/DataDog/datadog-agent/comp/otelcol/collector/impl"
	converter "github.com/DataDog/datadog-agent/comp/otelcol/converter/def"
	converterfx "github.com/DataDog/datadog-agent/comp/otelcol/converter/fx"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/logsagentpipelineimpl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	metricscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-otel"
	traceagentfx "github.com/DataDog/datadog-agent/comp/trace/agent/fx"
	traceagentcomp "github.com/DataDog/datadog-agent/comp/trace/agent/impl"
	gzipfx "github.com/DataDog/datadog-agent/comp/trace/compression/fx-gzip"
	traceconfig "github.com/DataDog/datadog-agent/comp/trace/config"
	payloadmodifierfx "github.com/DataDog/datadog-agent/comp/trace/payload-modifier/fx"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"

	"go.uber.org/fx"
)

type cliParams struct {
	*subcommands.GlobalParams

	// pidfilePath contains the value of the --pidfile flag.
	pidfilePath string
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

// A negative CMD_PORT is used to tell the otel-agent not to contact the core agent.
// e.g. in gateway mode
func isCmdPortNegative(cfg coreconfig.Component) bool {
	return cfg.GetInt("cmd_port") <= 0
}

func runOTelAgentCommand(ctx context.Context, params *cliParams, opts ...fx.Option) error {
	acfg, err := agentConfig.NewConfigComponent(context.Background(), params.CoreConfPath, params.ConfPaths)
	if err != nil && err != agentConfig.ErrNoDDExporter {
		return err
	}
	if !acfg.GetBool("otelcollector.enabled") {
		fmt.Println("*** OpenTelemetry Collector is not enabled, exiting application ***. Set the config option `otelcollector.enabled` or the environment variable `DD_OTELCOLLECTOR_ENABLED` at true to enable it.")
		return nil
	}

	uris := buildConfigURIs(params)

	if err == agentConfig.ErrNoDDExporter {
		return fxutil.Run(
			fx.Supply(uris),
			fx.Provide(func() coreconfig.Component {
				return acfg
			}),
			fx.Provide(func(_ coreconfig.Component) log.Params {
				return log.ForDaemon(params.LoggerName, "log_file", pkgconfigsetup.DefaultOTelAgentLogFile)
			}),
			logfx.Module(),
			ipcfx.ModuleReadWrite(),
			configsyncimpl.Module(configsyncimpl.NewParams(params.SyncTimeout, true, params.SyncOnInitTimeout)),
			pidfx.Module(),
			fx.Supply(pidimpl.NewParams(params.pidfilePath)),
			converterfx.Module(),
			fx.Provide(func(cp converter.Component, _ configsync.Component) confmap.Converter {
				return cp
			}),
			remoteTaggerFx.Module(tagger.OptionalRemoteParams{Disable: isCmdPortNegative}, tagger.NewRemoteParams()),
			fx.Provide(func(h hostnameinterface.Component) (serializerexporter.SourceProviderFunc, error) {
				return h.Get, nil
			}),
			telemetryimpl.Module(),
			remotehostnameimpl.Module(),
			collectorcontribFx.Module(),
			collectorfx.ModuleNoAgent(),
			fx.Options(opts...),
			fx.Invoke(func(_ collectordef.Component, _ pid.Component) {
			}),
			fxinstrumentation.Module(),
		)
	}

	return fxutil.Run(
		ForwarderBundle(),
		logtracefx.Module(),
		inventoryagentimpl.Module(),
		fx.Supply(metricsclient.NewStatsdClientWrapper(&ddgostatsd.NoOpClient{})),
		fx.Provide(func(client *metricsclient.StatsdClientWrapper) statsd.Component {
			return statsdotel.NewOTelStatsd(client)
		}),
		ipcfx.ModuleReadWrite(),
		collectorfx.Module(collectorimpl.NewParams(params.BYOC)),
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
		secretsnoopfx.Module(),
		workloadmetafx.Module(workloadmeta.Params{
			AgentType:  workloadmeta.NodeAgent,
			InitHelper: workloadmetainit.GetWorkloadmetaInit(),
		}),
		fx.Supply(uris),
		fx.Provide(func(h hostnameinterface.Component, cfg coreconfig.Component) (serializerexporter.SourceProviderFunc, error) {
			if cfg.GetBool("otelcollector.gateway.mode") {
				// In gateway mode the agent does not represent a specific host, so return an empty
				// hostname without error instead of failing when hostname resolution is not available.
				return func(_ context.Context) (string, error) {
					return "", nil
				}, nil
			}
			return h.Get, nil
		}),
		remotehostnameimpl.Module(),

		fx.Provide(func(_ coreconfig.Component) log.Params {
			return log.ForDaemon(params.LoggerName, "log_file", pkgconfigsetup.DefaultOTelAgentLogFile)
		}),
		fx.Provide(func() logconfig.IntakeOrigin {
			return logconfig.DDOTIntakeOrigin
		}),
		logsagentpipelineimpl.Module(),
		logscompressionfx.Module(),
		metricscompressionfx.Module(),
		// For FX to provide the compression.Compressor interface (used by serializer.NewSerializer)
		// implemented by the metricsCompression.Component
		fx.Provide(func(c metricscompression.Component) compression.Compressor {
			return c
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

		pidfx.Module(),
		fx.Supply(pidimpl.NewParams(params.pidfilePath)),
		fx.Provide(func(c defaultforwarder.Component) (defaultforwarder.Forwarder, error) {
			return defaultforwarder.Forwarder(c), nil
		}),
		fx.Provide(newOrchestratorinterfaceimpl),
		fx.Options(opts...),
		fx.Invoke(func(_ collectordef.Component, _ defaultforwarder.Forwarder, _ option.Option[logsagentpipeline.Component], _ pid.Component) {
		}),

		configsyncimpl.Module(configsyncimpl.NewParams(params.SyncTimeout, true, params.SyncOnInitTimeout)),
		remoteTaggerFx.Module(tagger.OptionalRemoteParams{Disable: isCmdPortNegative}, tagger.NewRemoteParams()),
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
		fx.Provide(func(deps traceconfig.Dependencies, _ configsync.Component) (traceconfig.Component, error) {
			// TODO: this would be much better if we could leverage traceconfig.Module
			//       Must add a new parameter to traconfig.Module to handle this.
			return traceconfig.NewConfig(deps)
		}),
		fx.Supply(traceconfig.Params{FailIfAPIKeyMissing: false}),

		fx.Supply(&traceagentcomp.Params{
			CPUProfile:               "",
			MemProfile:               "",
			PIDFilePath:              "",
			DisableInternalProfiling: true,
		}),
		payloadmodifierfx.NilModule(),
		traceagentfx.Module(),
		agenttelemetryfx.Module(),
		fxinstrumentation.Module(),
	)
}

// ForwarderBundle returns the fx.Option for the forwarder bundle.
// TODO: cleanup the forwarder instantiation with fx.
// This is a bit of a hack because we need to enforce configsync.Component
// is passed to newForwarder to enforce the correct instantiation order. Currently, the
// new forwarder.BundleWithProvider makes a few assumptions in its generic prototype, and
// this is the current workaround to leverage it.
func ForwarderBundle() fx.Option {
	return defaultforwarder.ModulWithOptionTMP(
		fx.Provide(func(_ configsync.Component) defaultforwarder.Params {
			return defaultforwarder.NewParams()
		}))
}

func buildConfigURIs(params *cliParams) []string {
	// Apply overrides
	uris := append([]string{}, params.ConfPaths...)

	// Add fleet policy config if DD_FLEET_POLICIES_DIR is set
	if fleetPoliciesDir := os.Getenv("DD_FLEET_POLICIES_DIR"); fleetPoliciesDir != "" {
		resolvedFleetPoliciesDir, err := filepath.EvalSymlinks(fleetPoliciesDir)
		if err != nil {
			if os.IsNotExist(err) {
				// Expected behavior
				fmt.Printf("Fleet policies directory does not exist: %s\n", fleetPoliciesDir)
			} else {
				fmt.Printf("Warning: failed to resolve symlinks for fleet policies dir %s: %v\n", fleetPoliciesDir, err)
			}
			resolvedFleetPoliciesDir = fleetPoliciesDir
		}

		// Make it absolute
		absFleetPoliciesDir, err := filepath.Abs(resolvedFleetPoliciesDir)
		if err != nil {
			fmt.Printf("Warning: failed to get absolute path for fleet policies dir %s: %v\n", resolvedFleetPoliciesDir, err)
			uris = append(uris, params.Sets...)
			return uris
		}

		fleetConfigPath := filepath.Join(absFleetPoliciesDir, "otel-config.yaml")

		_, err = os.Stat(fleetConfigPath)
		if err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: failed to access fleet policy config %s: %v\n", fleetConfigPath, err)
		}

		if err == nil {
			uris = append(uris, "file:"+fleetConfigPath)
			fmt.Printf("Using fleet policy config: %s\n", fleetConfigPath)
		}

	}

	uris = append(uris, params.Sets...)

	return uris
}
