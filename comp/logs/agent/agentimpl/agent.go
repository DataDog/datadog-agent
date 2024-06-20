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
	"time"

	"github.com/hashicorp/go-multierror"
	"go.uber.org/atomic"
	"go.uber.org/fx"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	statusComponent "github.com/DataDog/datadog-agent/comp/core/status"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	logsIntegrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/sds"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newLogsAgent))
}

type dependencies struct {
	fx.In

	Lc                 fx.Lifecycle
	Log                logComponent.Component
	Config             configComponent.Component
	InventoryAgent     inventoryagent.Component
	Hostname           hostname.Component
	WMeta              optional.Option[workloadmeta.Component]
	SchedulerProviders []schedulers.Scheduler `group:"log-agent-scheduler"`
	logsIntegrations   logsIntegrations.Component
}

type provides struct {
	fx.Out

	Comp           optional.Option[agent.Component]
	FlareProvider  flaretypes.Provider
	StatusProvider statusComponent.InformationProvider
	RCListener     rctypes.ListenerProvider
}

// logAgent represents the data pipeline that collects, decodes,
// processes and sends logs to the backend.  See the package README for
// a description of its operation.
type logAgent struct {
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
	schedulerProviders        []schedulers.Scheduler

	// started is true if the logs agent is running
	started *atomic.Bool
}

func newLogsAgent(deps dependencies) provides {
	if deps.Config.GetBool("logs_enabled") || deps.Config.GetBool("log_enabled") {
		if deps.Config.GetBool("log_enabled") {
			deps.Log.Warn(`"log_enabled" is deprecated, use "logs_enabled" instead`)
		}

		logsAgent := &logAgent{
			log:            deps.Log,
			config:         deps.Config,
			inventoryAgent: deps.InventoryAgent,
			hostname:       deps.Hostname,
			started:        atomic.NewBool(false),

			sources:            sources.NewLogSources(),
			services:           service.NewServices(),
			tracker:            tailers.NewTailerTracker(),
			flarecontroller:    flareController.NewFlareController(),
			wmeta:              deps.WMeta,
			schedulerProviders: deps.SchedulerProviders,
		}
		deps.Lc.Append(fx.Hook{
			OnStart: logsAgent.start,
			OnStop:  logsAgent.stop,
		})

		var rcListener rctypes.ListenerProvider
		if sds.SDSEnabled {
			rcListener.ListenerProvider = rctypes.RCListener{
				state.ProductSDSAgentConfig: logsAgent.onUpdateSDSAgentConfig,
				state.ProductSDSRules:       logsAgent.onUpdateSDSRules,
			}
		}

		return provides{
			Comp:           optional.NewOption[agent.Component](logsAgent),
			StatusProvider: statusComponent.NewInformationProvider(NewStatusProvider()),
			FlareProvider:  flaretypes.NewProvider(logsAgent.flarecontroller.FillFlare),
			RCListener:     rcListener,
		}
	}

	deps.Log.Info("logs-agent disabled")
	return provides{
		Comp:           optional.NewNoneOption[agent.Component](),
		StatusProvider: statusComponent.NewInformationProvider(NewStatusProvider()),
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
	a.log.Info("logs-agent started")

	for _, scheduler := range a.schedulerProviders {
		a.AddScheduler(scheduler)
	}

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

	a.SetupPipeline(processingRules, a.wmeta)
	return nil
}

// Start starts all the elements of the data pipeline
// in the right order to prevent data loss
func (a *logAgent) startPipeline() {
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

func (a *logAgent) onUpdateSDSRules(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) { //nolint:revive
	var err error
	for _, config := range updates {
		if rerr := a.pipelineProvider.ReconfigureSDSStandardRules(config.Config); rerr != nil {
			err = multierror.Append(err, rerr)
		}
	}

	if err != nil {
		log.Errorf("Can't update SDS standard rules: %v", err)
	}

	// Apply the new status to all configs
	for cfgPath := range updates {
		if err == nil {
			applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		} else {
			applyStateCallback(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		}
	}

}

func (a *logAgent) onUpdateSDSAgentConfig(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) { //nolint:revive
	var err error

	// We received a hit that new updates arrived, but if the list of updates
	// is empty, it means we don't have any updates applying to this agent anymore
	// Send a reconfiguration with an empty payload, indicating that
	// the scanners have to be dropped.
	if len(updates) == 0 {
		err = a.pipelineProvider.ReconfigureSDSAgentConfig([]byte("{}"))
	} else {
		for _, config := range updates {
			if rerr := a.pipelineProvider.ReconfigureSDSAgentConfig(config.Config); rerr != nil {
				err = multierror.Append(err, rerr)
			}
		}
	}

	if err != nil {
		log.Errorf("Can't update SDS configurations: %v", err)
	}

	// Apply the new status to all configs
	for cfgPath := range updates {
		if err == nil {
			applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		} else {
			applyStateCallback(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		}
	}
}
