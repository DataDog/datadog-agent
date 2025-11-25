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

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
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

	// Store current endpoints for rollback if HTTP restart fails
	previousEndpoints := a.endpoints

	a.log.Info("Gracefully stopping logs-agent")

	timeout := time.Duration(a.config.GetInt("logs_config.stop_grace_period")) * time.Second
	_, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := a.partialStop(); err != nil {
		a.log.Warn("Graceful partial stop timed out, force closing")
	}

	a.log.Info("Re-starting logs-agent...")

	// Build HTTP endpoints directly since we already verified HTTP connectivity
	// before triggering this restart (no need to recheck)
	endpoints, err := buildHTTPEndpointsForRestart(a.config)
	if err != nil {
		message := fmt.Sprintf("Invalid endpoints: %v", err)
		status.AddGlobalError(invalidEndpoints, message)
		a.log.Errorf("Failed to build HTTP endpoints, attempting rollback to previous transport: %v", err)
		return a.rollbackToPreviousTransport(previousEndpoints)
	}

	a.endpoints = endpoints

	err = a.setupAgentForRestart()
	if err != nil {
		message := fmt.Sprintf("Could not re-start logs-agent: %v", err)
		a.log.Error(message)
		a.log.Error("Attempting rollback to previous transport")
		return a.rollbackToPreviousTransport(previousEndpoints)
	}

	a.restartPipeline()
	return nil
}

// restartWithHTTPUpgrade upgrades the logs-agent pipeline to HTTP transport.
//
// Since HTTP connectivity was verified before calling this function, we commit to HTTP
// and will keep retrying HTTP even if the upgrade fails. If restart fails, the base
// restart() function will rollback to TCP temporarily, but this function returns an
// error to trigger retry - ensuring we eventually upgrade to HTTP since connectivity exists.
func (a *logAgent) restartWithHTTPUpgrade(ctx context.Context) error {
	err := a.restart(ctx)
	if err != nil {
		// Restart failed (may have rolled back to TCP to keep agent functional)
		// Since we verified HTTP connectivity, return error to trigger retry
		a.log.Warnf("HTTP upgrade attempt failed: %v - will retry on next attempt", err)
		return fmt.Errorf("HTTP upgrade failed: %w", err)
	}

	a.log.Info("Successfully upgraded to HTTP transport")
	return nil
}

// rollbackToPreviousTransport attempts to restore the agent to its previous working state
// after a failed transport switch. This ensures the agent continues functioning
// rather than being left in a broken state.
func (a *logAgent) rollbackToPreviousTransport(previousEndpoints *config.Endpoints) error {
	a.log.Warn("Rolling back to previous transport after failed restart")

	a.endpoints = previousEndpoints

	err := a.setupAgentForRestart()
	if err != nil {
		// This is a critical failure - we can't recover
		message := fmt.Sprintf("CRITICAL: Failed to rollback to previous transport: %v", err)
		a.log.Error(message)
		return errors.New(message)
	}

	a.restartPipeline()
	a.log.Info("Successfully rolled back to previous transport")
	return errors.New("restart failed, rolled back to previous transport")
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
