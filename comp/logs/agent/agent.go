// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/fx"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	statusComponent "github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

const (
	// key used to display a warning message on the agent status
	invalidProcessingRules = "invalid_global_processing_rules"
	invalidEndpoints       = "invalid_endpoints"
	intakeTrackType        = "logs"

	// Log messages
	multiLineWarning = "multi_line processing rules are not supported as global processing rules."

	// inventory setting name
	logsTransport = "logs_transport"
)

type dependencies struct {
	fx.In

	Lc             fx.Lifecycle
	Log            logComponent.Component
	Config         configComponent.Component
	InventoryAgent inventoryagent.Component
	Hostname       hostname.Component
	WMeta          optional.Option[workloadmeta.Component]
}

type provides struct {
	fx.Out

	Comp           optional.Option[Component]
	FlareProvider  flaretypes.Provider
	StatusProvider statusComponent.InformationProvider
}

// agent represents the data pipeline that collects, decodes,
// processes and sends logs to the backend.  See the package README for
// a description of its operation.
type agent struct {
	log            logComponent.Component
	config         pkgConfig.Reader
	inventoryAgent inventoryagent.Component
	hostname       hostname.Component

	sources                   *sources.LogSources
	services                  *service.Services
	endpoints                 *config.Endpoints
	tracker                   *tailers.TailerTracker
	schedulers                *schedulers.Schedulers
	auditor                   auditor.Auditor
	destinationsCtx           *client.DestinationsContext
	pipelineProvider          pipeline.Provider
	launchers                 *launchers.Launchers
	health                    *health.Handle
	diagnosticMessageReceiver *diagnostic.BufferedMessageReceiver
	flarecontroller           *flareController.FlareController
	wmeta                     optional.Option[workloadmeta.Component]

	// started is true if the logs agent is running
	started *atomic.Bool
}

func newLogsAgent(deps dependencies) provides {
	if deps.Config.GetBool("logs_enabled") || deps.Config.GetBool("log_enabled") {
		if deps.Config.GetBool("log_enabled") {
			deps.Log.Warn(`"log_enabled" is deprecated, use "logs_enabled" instead`)
		}

		logsAgent := &agent{
			log:            deps.Log,
			config:         deps.Config,
			inventoryAgent: deps.InventoryAgent,
			hostname:       deps.Hostname,
			started:        atomic.NewBool(false),

			sources:         sources.NewLogSources(),
			services:        service.NewServices(),
			tracker:         tailers.NewTailerTracker(),
			flarecontroller: flareController.NewFlareController(),
			wmeta:           deps.WMeta,
		}
		deps.Lc.Append(fx.Hook{
			OnStart: logsAgent.start,
			OnStop:  logsAgent.stop,
		})

		return provides{
			Comp:           optional.NewOption[Component](logsAgent),
			StatusProvider: statusComponent.NewInformationProvider(NewStatusProvider()),
			FlareProvider:  flaretypes.NewProvider(logsAgent.flarecontroller.FillFlare),
		}
	}

	deps.Log.Info("logs-agent disabled")
	return provides{
		Comp:           optional.NewNoneOption[Component](),
		StatusProvider: statusComponent.NewInformationProvider(NewStatusProvider()),
	}
}

func (a *agent) start(context.Context) error {
	a.log.Info("Starting logs-agent...")

	// setup the server config
	endpoints, err := buildEndpoints(a.config)

	if err != nil {
		message := fmt.Sprintf("Invalid endpoints: %v", err)
		status.AddGlobalError(invalidEndpoints, message)
		return errors.New(message)
	}

	a.endpoints = endpoints

	err = a.setupAgent()

	if err != nil {
		a.log.Error("Could not start logs-agent: ", err)
		return err
	}

	a.startPipeline()
	a.log.Info("logs-agent started")

	return nil
}

func (a *agent) setupAgent() error {
	if a.endpoints.UseHTTP {
		status.SetCurrentTransport(status.TransportHTTP)
	} else {
		status.SetCurrentTransport(status.TransportTCP)
	}
	// The severless agent doesn't use FX for now. This means that the logs agent will not have 'inventoryAgent'
	// initialized for serverless. This is ok since metadata is not enabled for serverless.
	// TODO: (components) - This condition should be removed once the serverless agent use FX.
	if a.inventoryAgent != nil {
		a.inventoryAgent.Set(logsTransport, string(status.GetCurrentTransport()))
	}

	// setup global processing rules
	processingRules, err := config.GlobalProcessingRules(a.config)
	if err != nil {
		message := fmt.Sprintf("Invalid processing rules: %v", err)
		status.AddGlobalError(invalidProcessingRules, message)
		return errors.New(message)
	}

	if config.HasMultiLineRule(processingRules) {
		a.log.Warn(multiLineWarning)
		status.AddGlobalWarning(invalidProcessingRules, multiLineWarning)
	}

	a.SetupPipeline(processingRules, a.wmeta)
	return nil
}

// Start starts all the elements of the data pipeline
// in the right order to prevent data loss
func (a *agent) startPipeline() {
	a.started.Store(true)

	// setup the status
	status.Init(a.started, a.endpoints, a.sources, a.tracker, metrics.LogsExpvars)

	starter := startstop.NewStarter(
		a.destinationsCtx,
		a.auditor,
		a.pipelineProvider,
		a.diagnosticMessageReceiver,
		a.launchers,
		a.schedulers,
	)
	starter.Start()
}

func (a *agent) stop(context.Context) error {
	a.log.Info("Stopping logs-agent")

	status.Clear()

	stopper := startstop.NewSerialStopper(
		a.schedulers,
		a.launchers,
		a.pipelineProvider,
		a.auditor,
		a.destinationsCtx,
		a.diagnosticMessageReceiver,
	)

	// This will try to stop everything in order, including the potentially blocking
	// parts like the sender. After StopTimeout it will just stop the last part of the
	// pipeline, disconnecting it from the auditor, to make sure that the pipeline is
	// flushed before stopping.
	// TODO: Add this feature in the stopper.
	c := make(chan struct{})
	go func() {
		stopper.Stop()
		close(c)
	}()
	timeout := time.Duration(a.config.GetInt("logs_config.stop_grace_period")) * time.Second
	select {
	case <-c:
	case <-time.After(timeout):
		a.log.Info("Timed out when stopping logs-agent, forcing it to stop now")
		// We force all destinations to read/flush all the messages they get without
		// trying to write to the network.
		a.destinationsCtx.Stop()
		// Wait again for the stopper to complete.
		// In some situation, the stopper unfortunately never succeed to complete,
		// we've already reached the grace period, give it some more seconds and
		// then force quit.
		timeout := time.NewTimer(5 * time.Second)
		select {
		case <-c:
		case <-timeout.C:
			a.log.Warn("Force close of the Logs Agent, dumping the Go routines.")
			if stack, err := util.GetGoRoutinesDump(); err != nil {
				a.log.Warnf("can't get the Go routines dump: %s\n", err)
			} else {
				a.log.Warn(stack)
			}
		}
	}
	a.log.Info("logs-agent stopped")
	return nil
}

// AddScheduler adds the given scheduler to the agent.
func (a *agent) AddScheduler(scheduler schedulers.Scheduler) {
	a.schedulers.AddScheduler(scheduler)
}

func (a *agent) GetSources() *sources.LogSources {
	return a.sources
}

func (a *agent) GetMessageReceiver() *diagnostic.BufferedMessageReceiver {
	return a.diagnosticMessageReceiver
}

func (a *agent) GetPipelineProvider() pipeline.Provider {
	return a.pipelineProvider
}
