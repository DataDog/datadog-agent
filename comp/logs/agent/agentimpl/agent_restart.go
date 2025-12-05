// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// restart conducts a partial restart of the logs-agent pipeline.
// This is used to switch between transport protocols
// without disrupting the entire agent.
func (a *logAgent) restart(context.Context) error {
	a.log.Info("Attempting to restart logs-agent pipeline")

	a.restartMutex.Lock()
	defer a.restartMutex.Unlock()

	a.log.Info("Gracefully stopping logs-agent")

	timeout := time.Duration(a.config.GetInt("logs_config.stop_grace_period")) * time.Second
	_, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := a.partialStop(); err != nil {
		a.log.Warn("Graceful partial stop timed out, force closing")
	}

	a.log.Info("Re-starting logs-agent...")

	endpoints, err := buildEndpoints(a.config)
	if err != nil {
		message := fmt.Sprintf("Invalid endpoints: %v", err)
		status.AddGlobalError(invalidEndpoints, message)
		return errors.New(message)
	}

	a.endpoints = endpoints

	err = a.setupAgentForRestart()
	if err != nil {
		message := fmt.Sprintf("Could not re-start logs-agent: %v", err)
		a.log.Error(message)
		return errors.New(message)
	}

	a.restartPipeline()
	return nil
}

// setupAgentForRestart configures and rebuilds only the transient components during a restart.
// This preserves persistent components (sources, auditor, tracker, schedulers)
// and only recreates components that need to be updated for the new configuration.
func (a *logAgent) setupAgentForRestart() error {
	processingRules, fingerprintConfig, err := a.configureAgent()
	if err != nil {
		return err
	}

	a.rebuildTransientComponents(processingRules, a.wmeta, a.integrationsLogs, *fingerprintConfig)
	return nil
}

// restartPipeline restarts the logs pipeline after a transport switch.
// Unlike startPipeline, this only starts the transient components (destinations, pipeline, launchers)
// since persistent components (auditor, schedulers, diagnosticMessageReceiver) remain running.
func (a *logAgent) restartPipeline() {
	status.Init(a.started, a.endpoints, a.sources, a.tracker, metrics.LogsExpvars)

	starter := startstop.NewStarter(a.destinationsCtx, a.pipelineProvider, a.launchers)
	starter.Start()

	a.log.Info("Successfully restarted pipeline")
}

// partialStop stops only the transient components that will be recreated during restart.
// This allows switching transports without losing persistent state.
//
// Components stopped (transient):
//   - launchers
//   - pipelineProvider
//   - destinationsCtx
//
// The partial stop ensures that log sources remain configured and file positions
// are maintained across the restart
func (a *logAgent) partialStop() error {
	a.log.Info("Completing graceful partial stop of logs-agent for restart")
	status.Clear()

	toStop := []startstop.Stoppable{
		a.launchers,
		a.pipelineProvider,
		a.destinationsCtx,
	}

	a.stopComponents(toStop, func() {
		a.destinationsCtx.Stop()
	})

	// Flush auditor to write current positions to disk
	a.log.Debug("Flushing auditor registry after pipeline stop")
	a.auditor.Flush()

	return nil
}
