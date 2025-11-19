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
	"os"
	"sync"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"

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
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/goroutinesdump"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

const (
	// key used to display a warning message on the agent status
	invalidProcessingRules   = "invalid_global_processing_rules"
	invalidEndpoints         = "invalid_endpoints"
	invalidFingerprintConfig = "invalid_fingerprint_config"
	intakeTrackType          = "logs"

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
	config         pkgconfigmodel.Reader
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

	// make restart thread safe
	restartMutex   sync.Mutex
	isShuttingDown atomic.Bool

	// ensure restart test only happens once per agent instance
	restartTestTriggered sync.Once

	// store start timings for comparison with restart
	startTimings struct {
		endpointsDuration time.Duration
		setupDuration     time.Duration
		pipelineDuration  time.Duration
		totalDuration     time.Duration
		recorded          bool
	}
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
	startTotal := time.Now()
	a.log.Info("Starting logs-agent...")

	if os.Getenv("DD_TEST_FORCE_TCP_AND_RESTART") == "1" {
		if cfg, ok := a.config.(pkgconfigmodel.Config); ok {
			// on start , force tcp ONCE
			cfg.Set("logs_config.test_restart_force_tcp", "1", pkgconfigmodel.SourceAgentRuntime)
		}
	}

	// setup the server config
	endpointsStart := time.Now()
	endpoints, err := buildEndpoints(a.config)

	if err != nil {
		message := fmt.Sprintf("Invalid endpoints: %v", err)
		status.AddGlobalError(invalidEndpoints, message)
		return errors.New(message)
	}

	a.endpoints = endpoints
	endpointsDuration := time.Since(endpointsStart)
	a.log.Debugf("[START-DUR] Endpoints built in %v", endpointsDuration)

	setupStart := time.Now()
	err = a.setupAgent()

	if err != nil {
		a.log.Error("Could not start logs-agent: ", err)
		return err
	}
	setupDuration := time.Since(setupStart)
	a.log.Debugf("[START-DUR] Agent setup completed in %v", setupDuration)

	pipelineStart := time.Now()
	a.startPipeline()
	pipelineDuration := time.Since(pipelineStart)
	totalStartDuration := time.Since(startTotal)

	// Store timings for comparison with restart
	a.startTimings.endpointsDuration = endpointsDuration
	a.startTimings.setupDuration = setupDuration
	a.startTimings.pipelineDuration = pipelineDuration
	a.startTimings.totalDuration = totalStartDuration
	a.startTimings.recorded = true

	a.log.Infof("[START-DUR] Start completed - endpoints: %v, setup: %v, pipeline: %v, total: %v", endpointsDuration, setupDuration, pipelineDuration, totalStartDuration)

	// force restart for local testing - only trigger once per agent instance
	if os.Getenv("DD_TEST_FORCE_TCP_AND_RESTART") == "1" {
		a.restartTestTriggered.Do(func() {
			// now to test restart , allow http endpoints to be built (no longer tcp)
			if cfg, ok := a.config.(pkgconfigmodel.Config); ok {
				cfg.Set("logs_config.test_restart_force_tcp", "0", pkgconfigmodel.SourceAgentRuntime)
			}
			go func() {
				// Wait longer to ensure sources are discovered and added by schedulers
				// Schedulers typically discover sources within 10-15 seconds
				time.Sleep(15 * time.Second)

				// Check if we have sources before restarting
				sourceCount := len(a.sources.GetSources())
				a.log.Infof("About to restart - found %d sources", sourceCount)

				_ = a.restart(context.Background())
			}()
		})
	}

	return nil
}

// restart conducts a partial restart of the logs-agent pipeline.
// This is used to switch between transport protocols (TCP to HTTP or vice versa)
// without disrupting the entire agent.
//
// The restart process:
// 1. Acquires a restart mutex to prevent concurrent restarts
// 2. Performs a partial stop of transient components (launchers, pipeline, destinations)
// 3. Rebuilds endpoints based on current configuration
// 4. Rebuilds transient components while preserving persistent state (sources, auditor, tracker, schedulers)
// 5. Restarts the pipeline with the new configuration
//
// Returns an error if the agent is shutting down, if endpoints are invalid,
// or if the restart setup fails.
func (a *logAgent) restart(context.Context) error {
	restartStart := time.Now()
	a.log.Info("Attempting to restart logs-agent pipeline with HTTP")

	a.restartMutex.Lock()
	defer a.restartMutex.Unlock()

	if a.isShuttingDown.Load() {
		return errors.New("agent shutting down")
	}

	stopStart := time.Now()
	a.log.Info("Gracefully stopping logs-agent")

	timeout := time.Duration(a.config.GetInt("logs_config.stop_grace_period")) * time.Second
	_, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := a.partialStop(); err != nil {
		a.log.Warn("Graceful partial stop timed out, force closing")
	}
	stopDuration := time.Since(stopStart)
	a.log.Infof("[RESTART-DUR] Stop phase completed in %v", stopDuration)

	rebuildStart := time.Now()
	a.log.Info("Re-starting logs-agent...")

	// REBUILD endpoints
	endpoints, err := buildEndpoints(a.config)
	if err != nil {
		message := fmt.Sprintf("Invalid endpoints: %v", err)
		status.AddGlobalError(invalidEndpoints, message)
		return errors.New(message)
	}

	a.endpoints = endpoints

	// REBUILD pipeline
	err = a.setupAgentForRestart()
	if err != nil {
		message := fmt.Sprintf("Could not restart logs-agent: %v", err)
		a.log.Error(message)
		return errors.New(message)
	}

	restartPipelineStart := time.Now()
	a.restartPipelineWithHTTP()
	restartPipelineDuration := time.Since(restartPipelineStart)

	rebuildDuration := time.Since(rebuildStart)
	totalDuration := time.Since(restartStart)
	a.log.Infof("[RESTART-DUR] Restart completed - stop: %v, rebuild: %v, pipeline: %v, total: %v", stopDuration, rebuildDuration, restartPipelineDuration, totalDuration)

	// Log comparison with start timings if available
	if a.startTimings.recorded {
		a.log.Infof("[COMPARISON] ========== Start vs Restart Timing Comparison ==========")
		a.log.Infof("[COMPARISON] Stop:       N/A        RESTART(stop)=%v  (restart overhead)", stopDuration)
		a.log.Infof("[COMPARISON] Endpoints:  START=%v  RESTART(rebuild)=%v  DIFF=%v",
			a.startTimings.endpointsDuration, rebuildDuration, rebuildDuration-a.startTimings.endpointsDuration)
		a.log.Infof("[COMPARISON] Setup:      START=%v  RESTART(rebuild)=%v  DIFF=%v",
			a.startTimings.setupDuration, rebuildDuration, rebuildDuration-a.startTimings.setupDuration)
		a.log.Infof("[COMPARISON] Pipeline:   START=%v  RESTART(pipeline)=%v  DIFF=%v",
			a.startTimings.pipelineDuration, restartPipelineDuration, restartPipelineDuration-a.startTimings.pipelineDuration)
		a.log.Infof("[COMPARISON] Total:      START=%v  RESTART=%v  DIFF=%v  (RESTART is %.1f%% of START)",
			a.startTimings.totalDuration, totalDuration, totalDuration-a.startTimings.totalDuration,
			float64(totalDuration)/float64(a.startTimings.totalDuration)*100)
		a.log.Infof("[COMPARISON] Breakdown: RESTART = stop(%v) + rebuild(%v) + pipeline(%v)",
			stopDuration, rebuildDuration, restartPipelineDuration)
		if totalDuration < a.startTimings.totalDuration {
			a.log.Infof("[COMPARISON] ✓ Restart is %.1f%% FASTER than start",
				(1.0-float64(totalDuration)/float64(a.startTimings.totalDuration))*100)
		} else {
			a.log.Infof("[COMPARISON] ⚠ Restart is %.1f%% SLOWER than start (includes %v stop overhead)",
				(float64(totalDuration)/float64(a.startTimings.totalDuration)-1.0)*100, stopDuration)
		}
		a.log.Infof("[COMPARISON] =========================================================")
	}
	return nil
}

func (a *logAgent) setupAgent() error {
	processingRules, fingerprintConfig, err := a.configureAgent()
	if err != nil {
		return err
	}

	a.SetupPipeline(processingRules, a.wmeta, a.integrationsLogs, *fingerprintConfig)

	return nil
}

// setupAgentForRestart configures and rebuilds only the transient components during a restart.
// Unlike setupAgent, this preserves persistent components (sources, auditor, tracker, schedulers)
// and only recreates components that need to be updated for the new configuration.
//
// Returns an error if configuration validation fails.
func (a *logAgent) setupAgentForRestart() error {
	processingRules, fingerprintConfig, err := a.configureAgent()
	if err != nil {
		return err
	}

	a.rebuildTransientComponents(processingRules, a.wmeta, a.integrationsLogs, *fingerprintConfig)
	return nil
}

// configureAgent validates and retrieves configuration settings needed for agent operation.
// This includes:
//   - Setting the current transport (HTTP or TCP) in the status
//   - Updating inventory metadata with transport information
//   - Validating and retrieving global log processing rules
//   - Validating and retrieving fingerprint configuration for file tailing
//
// Returns the processing rules and fingerprint config, or an error if validation fails.
func (a *logAgent) configureAgent() ([]*config.ProcessingRule, *types.FingerprintConfig, error) {
	if a.endpoints.UseHTTP {
		a.log.Debugf("configureAgent: transport=HTTP")
		status.SetCurrentTransport(status.TransportHTTP)
	} else {
		status.SetCurrentTransport(status.TransportTCP)
		a.log.Debugf("configureAgent: transport=TCP")
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
		return nil, nil, errors.New(message)
	}

	if config.HasMultiLineRule(processingRules) {
		a.log.Warn(multiLineWarning)
		status.AddGlobalWarning(invalidProcessingRules, multiLineWarning)
	}

	fingerprintConfig, err := config.GlobalFingerprintConfig(a.config)
	if err != nil {
		message := fmt.Sprintf("Invalid fingerprinting config: %v", err)
		status.AddGlobalError(invalidFingerprintConfig, message)
		return nil, nil, errors.New(message)
	}

	return processingRules, fingerprintConfig, nil
}

// Start starts all the elements of the data pipeline
// in the right order to prevent data loss
func (a *logAgent) startPipeline() {
	pipelineStartTotal := time.Now()

	// setup the status
	statusInitStart := time.Now()
	status.Init(a.started, a.endpoints, a.sources, a.tracker, metrics.LogsExpvars)
	statusInitDuration := time.Since(statusInitStart)
	a.log.Debugf("[START_PIPELINE-DUR] Status initialized in %v", statusInitDuration)

	starterStart := time.Now()
	starter := startstop.NewStarter(
		a.destinationsCtx,
		a.auditor,
		a.pipelineProvider,
		a.diagnosticMessageReceiver,
		a.launchers,
	)
	starter.Start()
	starterDuration := time.Since(starterStart)
	a.log.Debugf("[START_PIPELINE-DUR] Pipeline components started in %v", starterDuration)

	schedulersStart := time.Now()
	a.startSchedulers()
	schedulersDuration := time.Since(schedulersStart)
	pipelineTotalDuration := time.Since(pipelineStartTotal)
	a.log.Debugf("[START_PIPELINE-DUR] Pipeline start completed - status: %v, components: %v, schedulers: %v, total: %v", statusInitDuration, starterDuration, schedulersDuration, pipelineTotalDuration)
}

// restartPipelineWithHTTP restarts the logs pipeline after a transport switch.
// Unlike startPipeline, this only starts the transient components (destinations, pipeline, launchers)
// since persistent components (auditor, schedulers, diagnosticMessageReceiver) remain running.
func (a *logAgent) restartPipelineWithHTTP() {
	restartPipelineStart := time.Now()

	statusInitStart := time.Now()
	status.Init(a.started, a.endpoints, a.sources, a.tracker, metrics.LogsExpvars)
	statusInitDuration := time.Since(statusInitStart)
	a.log.Debugf("[RESTART-DUR] Status re-initialized in %v", statusInitDuration)

	// Log source count before restart for debugging
	sourceCount := len(a.sources.GetSources())
	a.log.Infof("Restarting pipeline with %d existing sources", sourceCount)

	starterStart := time.Now()
	starter := startstop.NewStarter(a.destinationsCtx, a.pipelineProvider, a.launchers)
	starter.Start()
	starterDuration := time.Since(starterStart)
	a.log.Debugf("[RESTART-DUR] Restart pipeline components started in %v", starterDuration)

	totalRestartPipelineDuration := time.Since(restartPipelineStart)
	a.log.Info("Successfully restarted pipeline with HTTP")
	a.log.Debugf("[RESTART-DUR] Restart pipeline total duration: %v (status: %v, components: %v)", totalRestartPipelineDuration, statusInitDuration, starterDuration)

	// Log source count after restart for debugging
	sourceCountAfter := len(a.sources.GetSources())
	a.log.Infof("After restart, %d sources available", sourceCountAfter)
}

func (a *logAgent) startSchedulers() {
	a.prepareSchedulers.Do(func() {
		a.schedulers.Start()

		for _, scheduler := range a.schedulerProviders {
			a.AddScheduler(scheduler)
		}

		a.started.Store(status.StatusRunning)
		a.log.Info("logs-agent started")
	})
}

func (a *logAgent) stop(context.Context) error {
	a.log.Info("Stopping logs-agent")

	status.Clear()

	toStop := []startstop.Stoppable{
		a.schedulers,
		a.launchers,
		a.pipelineProvider,
		a.auditor,
		a.destinationsCtx,
		a.diagnosticMessageReceiver,
	}

	a.stopComponents(toStop, func() {
		a.destinationsCtx.Stop()
	})

	return nil
}

// partialStop stops only the transient components that will be recreated during restart.
// This allows switching transports without losing persistent state.
//
// Components stopped (transient):
//   - launchers
//   - pipelineProvider
//   - destinationsCtx
//
// Components preserved (persistent):
//   - auditor
//   - sources
//   - tracker
//   - schedulers
//   - diagnosticMessageReceiver
//
// The partial stop ensures that log sources remain configured and file positions
// are maintained across the restart, allowing seamless continuation of log collection
// with the new transport.
func (a *logAgent) partialStop() error {
	partialStopStart := time.Now()
	a.log.Info("Completing graceful partial stop of logs-agent for restart")
	status.Clear()

	toStop := []startstop.Stoppable{
		a.launchers,
		a.pipelineProvider,
		a.destinationsCtx,
	}

	stopComponentsStart := time.Now()
	a.stopComponents(toStop, func() {
		a.destinationsCtx.Stop()
	})
	stopComponentsDuration := time.Since(stopComponentsStart)
	a.log.Debugf("Components stopped in %v", stopComponentsDuration)

	// Immediately flush auditor to write current positions to disk
	// TODO: this enables at-least-once delivery during restart (1-2 logs may be re-read)
	flushStart := time.Now()
	a.log.Debug("Flushing auditor registry after pipeline stop")
	a.auditor.Flush()
	flushDuration := time.Since(flushStart)
	a.log.Debugf("[RESTART-DUR] Auditor flush completed in %v", flushDuration)

	totalPartialStopDuration := time.Since(partialStopStart)
	a.log.Debugf("[RESTART-DUR] Partial stop total duration: %v (components: %v, flush: %v)", totalPartialStopDuration, stopComponentsDuration, flushDuration)
	return nil
}

// stopComponents stops the provided components using SerialStopper with a grace period timeout.
// The stop process:
// 1. Attempts graceful shutdown within the configured stop_grace_period
// 2. If timeout expires, calls forceClose to force-flush pending data
// 3. Waits an additional 5 seconds for cleanup
// 4. If still not complete, dumps goroutines for debugging and exits
func (a *logAgent) stopComponents(components []startstop.Stoppable, forceClose func()) {
	stopper := startstop.NewSerialStopper(components...)

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
		a.log.Debug("Components stopped gracefully")
	case <-time.After(timeout):
		a.log.Info("Timed out when stopping logs-agent, forcing it to stop now")
		// We force all destinations to read/flush all the messages they get without
		// trying to write to the network.
		if forceClose != nil {
			forceClose()
		}
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
