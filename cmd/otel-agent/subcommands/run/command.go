// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package run

import (
	"context"

	agentConfig "github.com/DataDog/datadog-agent/cmd/otel-agent/config"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	corelogimpl "github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/log/tracelogimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	collectorcontribFx "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/fx"
	collectordef "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	collectorfx "github.com/DataDog/datadog-agent/comp/otelcol/collector/fx"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/logsagentpipelineimpl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	configprovider "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/pipeline/provider"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl/strategy"
	tracecomp "github.com/DataDog/datadog-agent/comp/trace"
	traceagentcomp "github.com/DataDog/datadog-agent/comp/trace/agent"
	traceconfig "github.com/DataDog/datadog-agent/comp/trace/config"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	pkgtraceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/otelcol"

	"go.uber.org/fx"
)

// MakeCommand creates the `run` command
func MakeCommand(globalConfGetter func() *subcommands.GlobalParams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Starting OpenTelemetry Collector",
		RunE: func(cmd *cobra.Command, args []string) error {
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

type remotehostimpl struct {
	hostname string
}

func (r *remotehostimpl) Get(ctx context.Context) (string, error) {
	if r.hostname != "" {
		return r.hostname, nil
	}
	hostname, err := utils.GetHostnameWithContextAndFallback(ctx)
	if err != nil {
		return "", err
	}
	r.hostname = hostname
	return hostname, nil
}

func runOTelAgentCommand(ctx context.Context, params *subcommands.GlobalParams, opts ...fx.Option) error {
	err := fxutil.Run(
		forwarder.Bundle(),
		tracelogimpl.Module(), // cannot use corelogimpl and tracelogimpl at the same time
		inventoryagentimpl.Module(),
		workloadmeta.Module(),
		hostnameimpl.Module(),
		statsd.Module(),
		sysprobeconfig.NoneModule(),
		fetchonlyimpl.Module(),
		collectorfx.Module(),
		collectorcontribFx.Module(),
		fx.Provide(configprovider.NewConfigProvider),
		// For FX to provide the otelcol.ConfigProvider from the configprovider.ExtendedConfigProvider
		fx.Provide(func(cp configprovider.ExtendedConfigProvider) otelcol.ConfigProvider {
			return cp
		}),
		fx.Provide(func() (coreconfig.Component, *pkgtraceconfig.AgentConfig, error) {
			c, tcfg, err := agentConfig.NewConfigComponent(context.Background(), params.ConfPaths)
			if err != nil {
				return nil, nil, err
			}
			pkgconfigenv.DetectFeatures(c)
			return c, tcfg, nil
		}),

		fx.Provide(func(c statsd.Component) (ddgostatsd.ClientInterface, error) {
			return c.Get()
		}),
		// TODO: remove this
		fx.Provide(func(tcfg *pkgtraceconfig.AgentConfig, statsdClient ddgostatsd.ClientInterface) (*pkgagent.Agent, error) {
			return pkgagent.NewAgent(ctx, tcfg, telemetry.NewCollector(tcfg), statsdClient), nil
		}),

		fx.Provide(func() workloadmeta.Params {
			return workloadmeta.NewParams()
		}),
		fx.Provide(func() []string {
			return append(params.ConfPaths, params.Sets...)
		}),
		fx.Provide(func() (serializerexporter.SourceProviderFunc, error) {
			rh := &remotehostimpl{}
			return rh.Get, nil
		}),

		fx.Supply(optional.NewNoneOption[secrets.Component]()),
		fx.Provide(func(c coreconfig.Component) corelogimpl.Params {
			return corelogimpl.ForDaemon(params.LoggerName, "log_file", pkgconfigsetup.DefaultOTelAgentLogFile)
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
		fx.Provide(func(h serializerexporter.SourceProviderFunc) (string, error) {
			hn, err := h(context.Background())
			if err != nil {
				return "", err
			}
			log.Info("Using ", "hostname", hn)

			return hn, nil
		}),

		fx.Provide(newForwarderParams),
		fx.Provide(func(c defaultforwarder.Component) (defaultforwarder.Forwarder, error) {
			return defaultforwarder.Forwarder(c), nil
		}),
		fx.Provide(newOrchestratorinterfaceimpl),
		fx.Options(opts...),
		fx.Invoke(func(_ collectordef.Component, _ defaultforwarder.Forwarder, _ optional.Option[logsagentpipeline.Component]) {
		}),

		// ctx is required to be supplied from here, as Windows needs to inject its own context
		// to allow the agent to work as a service.
		fx.Provide(func() context.Context { return ctx }), // fx.Supply(ctx) fails with a missing type error.
		fx.Supply(&traceagentcomp.Params{
			CPUProfile:  "",
			MemProfile:  "",
			PIDFilePath: "",
		}),
		tracecomp.Bundle(),
		// fx.Provide(func(c traceagentcomp.Component) *pkgagent.Agent {
		// 	return c.GetAgent()
		// }),
		fx.Provide(func(coreConfig coreconfig.Component) tagger.Params {
			if coreConfig.GetBool("apm_config.remote_tagger") {
				return tagger.NewNodeRemoteTaggerParamsWithFallback()
			}
			return tagger.NewTaggerParams()
		}),
		taggerimpl.Module(),
		fx.Provide(func(cfg traceconfig.Component) telemetry.TelemetryCollector {
			return telemetry.NewCollector(cfg.Object())
		}),
	)
	if err != nil {
		return err
	}
	return nil
}

func newForwarderParams(config coreconfig.Component, log corelog.Component) defaultforwarder.Params {
	return defaultforwarder.NewParams(config, log)
}
