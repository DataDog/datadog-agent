// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/mini-agent/subcommands"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	taggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	workloadmetainit "github.com/DataDog/datadog-agent/comp/core/workloadmeta/init"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	metricscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	*subcommands.GlobalParams
	pidfilePath string
}

// MakeCommand creates the run command
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParamsGetter(),
	}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the mini-agent",
		Long:  `Run the mini-agent with tagger server and metric submission capabilities.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMiniAgent(context.Background(), cliParams)
		},
	}

	cmd.Flags().StringVar(&cliParams.pidfilePath, "pidfile", "", "Path to PID file")

	return cmd
}

// isCmdPortNegative checks if cmd_port is negative (used to disable remote tagger)
func isCmdPortNegative(cfg coreconfig.Component) bool {
	return cfg.GetInt("cmd_port") <= 0
}

// orchestratorinterfaceimpl is a simple implementation of orchestratorinterface.Component
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

func runMiniAgent(ctx context.Context, params *cliParams) error {
	return fxutil.Run(
		// Provide context
		fx.Provide(func() context.Context { return ctx }),

		// Supply config params
		fx.Supply(coreconfig.NewAgentParams(params.ConfPath)),
		fx.Supply(pidimpl.NewParams(params.pidfilePath)),

		// Supply log params
		fx.Supply(log.ForDaemon(params.LoggerName, "log_file", "/var/log/datadog/mini-agent.log")),

		// Config component
		coreconfig.Module(),

		// Logging
		logfx.Module(),

		// Secrets (no-op)
		secretsnoopfx.Module(),

		// IPC for tagger server authentication
		ipcfx.ModuleReadWrite(),

		// PID file
		pidimpl.Module(),

		// Hostname
		remotehostnameimpl.Module(),

		// Telemetry
		telemetryimpl.Module(),

		// Workloadmeta (required by tagger)
		workloadmetafx.Module(workloadmeta.Params{
			AgentType:  workloadmeta.NodeAgent,
			InitHelper: workloadmetainit.GetWorkloadmetaInit(),
		}),

		// Tagger - use local tagger implementation for mini-agent since it serves as tagger server
		taggerfx.Module(),

		// Forwarder for metrics
		defaultforwarder.Module(defaultforwarder.NewParams()),
		fx.Provide(func(c defaultforwarder.Component) (defaultforwarder.Forwarder, error) {
			return defaultforwarder.Forwarder(c), nil
		}),
		fx.Provide(newOrchestratorinterfaceimpl),

		// Serializer for metrics
		metricscompressionfx.Module(),
		fx.Provide(func(c metricscompression.Component) compression.Compressor {
			return c
		}),
		fx.Provide(func(h hostnameinterface.Component, l log.Component) (string, error) {
			hn, err := h.Get(context.Background())
			if err != nil {
				return "", err
			}
			l.Infof("Using hostname: %s", hn)
			return hn, nil
		}),
		fx.Provide(serializer.NewSerializer),
		fx.Provide(func(s *serializer.Serializer) serializer.MetricSerializer {
			return s
		}),

		// Mini-agent component
		fx.Provide(newMiniAgent),

		// Invoke the mini-agent to start it
		fx.Invoke(func(m *miniAgent, _ pid.Component) error {
			return m.start()
		}),
	)
}
