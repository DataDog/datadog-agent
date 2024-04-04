// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	corelogimpl "github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface"
	"github.com/DataDog/datadog-agent/comp/logs"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/comp/otelcol"
	"github.com/DataDog/datadog-agent/comp/otelcol/collector"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/spf13/cobra"

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

func runOTelAgentCommand(_ context.Context, params *subcommands.GlobalParams) error {
	err := fxutil.Run(
		forwarder.Bundle(),
		otelcol.Bundle(),
		// TODO: remove this once we start reading the collector config
		// We need to create a new config module from the collector config
		config.Module(),
		corelogimpl.Module(),
		inventoryagentimpl.Module(),
		workloadmeta.Module(),
		hostnameimpl.Module(),
		sysprobeconfig.NoneModule(),
		fetchonlyimpl.Module(),

		fx.Provide(func() workloadmeta.Params {
			return workloadmeta.NewParams()
		}),

		// TODO: remove this once we start reading the collector config
		fx.Provide(func() config.Params {
			return config.NewAgentParams(params.ConfPath)
		}),
		fx.Provide(func() corelogimpl.Params {
			// TODO configure the log level from collector config
			return corelogimpl.ForOneShot(params.LoggerName, "debug", true)
		}),
		logs.Bundle(),
		fx.Provide(serializer.NewSerializer),
		// For FX to provide the serializer.MetricSerializer from the serializer.Serializer
		fx.Provide(func(s *serializer.Serializer) serializer.MetricSerializer {
			return s
		}),
		fx.Provide(func(h hostname.Component) string {
			hn, _ := h.Get(context.Background())
			return hn
		}),

		fx.Provide(newForwarderParams),
		fx.Provide(func(c defaultforwarder.Component) (defaultforwarder.Forwarder, error) {
			return defaultforwarder.Forwarder(c), nil
		}),
		fx.Provide(newOrchestratorinterfaceimpl),
		fx.Invoke(func(_ collector.Component, _ defaultforwarder.Forwarder, _ optional.Option[logsAgent.Component]) {
		}),
	)
	if err != nil {
		return err
	}
	return nil
}

func newForwarderParams(config config.Component, log corelog.Component) defaultforwarder.Params {
	return defaultforwarder.NewParams(config, log)
}
