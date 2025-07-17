// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package agentimpl contains the implementation of the logs agent component.
package agentimpl

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	apiutils "github.com/DataDog/datadog-agent/comp/api/api/utils/stream"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	statusComponent "github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	integrationsimpl "github.com/DataDog/datadog-agent/comp/logs/integrations/impl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
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
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/goroutinesdump"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newLogsAgent))
}

type dependencies struct {
	fx.In

	Lc                 fx.Lifecycle
	Log                log.Component
	Config             configComponent.Component
	InventoryAgent     inventoryagent.Component
	Hostname           hostname.Component
	Auditor            auditor.Component
	WMeta              option.Option[workloadmeta.Component]
	SchedulerProviders []schedulers.Scheduler `group:"log-agent-scheduler"`
	Tagger             tagger.Component
	Compression        logscompression.Component
}

type provides struct {
	fx.Out

	Comp           option.Option[agent.Component]
	FlareProvider  flaretypes.Provider
	StatusProvider statusComponent.InformationProvider
	LogsReciever   option.Option[integrations.Component]
	APIStreamLogs  api.AgentEndpointProvider
}

// logAgent represents the data pipeline that collects, decodes,
// processes and sends logs to the backend.  See the package README for
// a description of its operation.
type logAgent struct {
	log            log.Component
	config         model.Reader
	inventoryAgent inventoryagent.Component
	hostname       hostname.Component
	tagger         tagger.Component

	sources                   *sources.LogSources
	services                  *service.Services
	endpoints                 *config.Endpoints
	tracker                   *tailers.TailerTracker
	schedulers                *schedulers.Schedulers
	auditor                   auditor.Component
	destinationsCtx           *client.DestinationsContext
	pipelineProvider          pipeline.Provider
	launchers                 *launchers.Launchers
	diagnosticMessageReceiver *diagnostic.BufferedMessageReceiver
	flarecontroller           *flareController.FlareController
	wmeta                     option.Option[workloadmeta.Component]
	schedulerProviders        []schedulers.Scheduler
	integrationsLogs          integrations.Component
	compression               logscompression.Component

	// make sure this is done only once, when we're ready
	prepareSchedulers sync.Once

	// started is true if the logs agent is running
	started *atomic.Uint32
}

func newLogsAgent(deps dependencies) provides {
	if deps.Config.GetBool("logs_enabled") || deps.Config.GetBool("log_enabled") {
		if deps.Config.GetBool("log_enabled") {
			deps.Log.Warn(`"log_enabled" is deprecated, use "logs_enabled" instead`)
		}

		integrationsLogs := integrationsimpl.NewLogsIntegration()

		logsAgent := &logAgent{
			log:                deps.Log,
			config:             deps.Config,
			inventoryAgent:     deps.InventoryAgent,
			hostname:           deps.Hostname,
			started:            atomic.NewUint32(status.StatusNotStarted),
			auditor:            deps.Auditor,
			sources:            sources.NewLogSources(),
			services:           service.NewServices(),
			tracker:            tailers.NewTailerTracker(),
			flarecontroller:    flareController.NewFlareController(),
			wmeta:              deps.WMeta,
			schedulerProviders: deps.SchedulerProviders,
			integrationsLogs:   integrationsLogs,
			tagger:             deps.Tagger,
			compression:        deps.Compression,
		}
		deps.Lc.Append(fx.Hook{
			OnStart: logsAgent.start,
			OnStop:  logsAgent.stop,
		})

		return provides{
			Comp:           option.New[agent.Component](logsAgent),
			StatusProvider: statusComponent.NewInformationProvider(NewStatusProvider()),
			FlareProvider:  flaretypes.NewProvider(logsAgent.flarecontroller.FillFlare),
			LogsReciever:   option.New[integrations.Component](integrationsLogs),
			APIStreamLogs: api.NewAgentEndpointProvider(streamLogsEvents(logsAgent),
				"/stream-logs",
				"POST",
			),
		}
	}

	deps.Log.Info("logs-agent disabled")
	return provides{
		Comp:           option.None[agent.Component](),
		StatusProvider: statusComponent.NewInformationProvider(NewStatusProvider()),
		LogsReciever:   option.None[integrations.Component](),
	}
}

func (a *logAgent) start(context.Context) error {
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
	return nil
}

func (a *logAgent) setupAgent() error {
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

	a.SetupPipeline(processingRules, a.wmeta, a.integrationsLogs)
	return nil
}

// Start starts all the elements of the data pipeline
// in the right order to prevent data loss
func (a *logAgent) startPipeline() {

	// setup the status
	status.Init(a.started, a.endpoints, a.sources, a.tracker, metrics.LogsExpvars)

	starter := startstop.NewStarter(
		a.destinationsCtx,
		a.auditor,
		a.pipelineProvider,
		a.diagnosticMessageReceiver,
		a.launchers,
	)
	starter.Start()
	a.startSchedulers()
}

func (a *logAgent) startSchedulers() {
	a.prepareSchedulers.Do(func() {
		a.schedulers.Start()

		for _, scheduler := range a.schedulerProviders {
			a.AddScheduler(scheduler)
		}

		a.log.Info("logs-agent started")
		a.started.Store(status.StatusRunning)
	})
}

func (a *logAgent) stop(context.Context) error {
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
			if stack, err := goroutinesdump.Get(); err != nil {
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
func (a *logAgent) AddScheduler(scheduler schedulers.Scheduler) {
	a.schedulers.AddScheduler(scheduler)
}

func (a *logAgent) GetSources() *sources.LogSources {
	return a.sources
}

func (a *logAgent) GetMessageReceiver() *diagnostic.BufferedMessageReceiver {
	return a.diagnosticMessageReceiver
}

func (a *logAgent) GetPipelineProvider() pipeline.Provider {
	return a.pipelineProvider
}

func streamLogsEvents(logsAgent agent.Component) func(w http.ResponseWriter, r *http.Request) {
	return apiutils.GetStreamFunc(func() apiutils.MessageReceiver {
		return logsAgent.GetMessageReceiver()
	}, "logs", "logs agent")
}
