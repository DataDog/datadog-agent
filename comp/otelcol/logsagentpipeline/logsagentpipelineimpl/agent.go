// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package logsagentpipelineimpl

import (
	"context"
	"errors"
	"fmt"
	"time"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	compression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	intakeTrackType = "logs"

	// Log messages
	multiLineWarning = "multi_line processing rules are not supported as global processing rules."
)

// Dependencies specifies the list of dependencies needed to initialize the logs agent
type Dependencies struct {
	fx.In

	Lc           fx.Lifecycle
	Log          log.Component
	Config       configComponent.Component
	Hostname     hostnameinterface.Component
	Compression  compression.Component
	IntakeOrigin config.IntakeOrigin
}

// Agent represents the data pipeline that collects, decodes, processes and sends logs to the backend.
type Agent struct {
	log          log.Component
	config       pkgconfigmodel.Reader
	hostname     hostnameinterface.Component
	compression  compression.Component
	intakeOrigin config.IntakeOrigin

	endpoints        *config.Endpoints
	destinationsCtx  *client.DestinationsContext
	pipelineProvider pipeline.Provider
}

// NewLogsAgentComponent returns a new instance of Agent as a Component
func NewLogsAgentComponent(deps Dependencies) option.Option[logsagentpipeline.Component] {
	logsAgent := NewLogsAgent(deps)
	if logsAgent == nil {
		return option.None[logsagentpipeline.Component]()
	}
	return option.New[logsagentpipeline.Component](logsAgent)
}

// NewLogsAgent returns a new instance of Agent with the given dependencies
func NewLogsAgent(deps Dependencies) logsagentpipeline.LogsAgent {
	if deps.Config.GetBool("logs_enabled") || deps.Config.GetBool("log_enabled") {
		if deps.Config.GetBool("log_enabled") {
			deps.Log.Warn(`"log_enabled" is deprecated, use "logs_enabled" instead`)
		}

		logsAgent := &Agent{
			log:          deps.Log,
			config:       deps.Config,
			hostname:     deps.Hostname,
			compression:  deps.Compression,
			intakeOrigin: deps.IntakeOrigin,
		}
		if deps.Lc != nil {
			deps.Lc.Append(fx.Hook{
				OnStart: logsAgent.Start,
				OnStop:  logsAgent.Stop,
			})
		}

		return logsAgent
	}

	deps.Log.Debug("logs-agent disabled")
	return nil
}

// Start sets up the logs agent and starts its pipelines
func (a *Agent) Start(context.Context) error {
	a.log.Debug("Starting logs-agent...")

	// setup the server config
	endpoints, err := buildEndpoints(a.config, a.log, a.intakeOrigin)

	if err != nil {
		message := fmt.Sprintf("Invalid endpoints: %v", err)
		return errors.New(message)
	}

	a.endpoints = endpoints

	err = a.setupAgent()

	if err != nil {
		a.log.Error("Could not start logs-agent: ", zap.Error(err))
		return err
	}

	a.startPipeline()
	a.log.Debug("logs-agent started")

	return nil
}

func (a *Agent) setupAgent() error {
	// setup global processing rules
	processingRules, err := config.GlobalProcessingRules(a.config)
	if err != nil {
		message := fmt.Sprintf("Invalid processing rules: %v", err)
		return errors.New(message)
	}

	if config.HasMultiLineRule(processingRules) {
		a.log.Warn(multiLineWarning)
	}

	a.SetupPipeline(processingRules)
	return nil
}

// startPipeline starts all the elements of the data pipeline in the right order to prevent data loss
func (a *Agent) startPipeline() {
	starter := startstop.NewStarter(
		a.destinationsCtx,
		a.pipelineProvider,
	)
	starter.Start()
}

// Stop stops the logs agent and all elements of the data pipeline
func (a *Agent) Stop(context.Context) error {
	a.log.Debug("Stopping logs-agent")

	stopper := startstop.NewSerialStopper(
		a.pipelineProvider,
		a.destinationsCtx,
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
		a.log.Debug("Timed out when stopping logs-agent, forcing it to stop now")
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
			a.log.Warn("Force close of the Logs Agent.")
		}
	}
	a.log.Debug("logs-agent stopped")
	return nil
}

// GetPipelineProvider gets the pipeline provider
func (a *Agent) GetPipelineProvider() pipeline.Provider {
	return a.pipelineProvider
}

// SetupPipeline initializes the logs agent pipeline and its dependencies
func (a *Agent) SetupPipeline(
	processingRules []*config.ProcessingRule,
) {
	destinationsCtx := client.NewDestinationsContext()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(
		a.config.GetInt("logs_config.pipelines"),
		&sender.NoopSink{},
		&diagnostic.NoopMessageReceiver{},
		processingRules,
		a.endpoints,
		destinationsCtx,
		NewStatusProvider(),
		a.hostname,
		a.config,
		a.compression,
		a.config.GetBool("logs_config.disable_distributed_senders"),
		false, // serverless
	)

	a.destinationsCtx = destinationsCtx
	a.pipelineProvider = pipelineProvider
}

// buildEndpoints builds endpoints for the logs agent
func buildEndpoints(coreConfig pkgconfigmodel.Reader, log log.Component, intakeOrigin config.IntakeOrigin) (*config.Endpoints, error) {
	httpConnectivity := config.HTTPConnectivityFailure
	if endpoints, err := config.BuildHTTPEndpoints(coreConfig, intakeTrackType, config.AgentJSONIntakeProtocol, intakeOrigin); err == nil {
		httpConnectivity = http.CheckConnectivity(endpoints.Main, coreConfig)
		if !httpConnectivity {
			log.Warn("Error while validating API key")
		}
	}
	return config.BuildEndpoints(coreConfig, httpConnectivity, intakeTrackType, config.AgentJSONIntakeProtocol, intakeOrigin)
}
