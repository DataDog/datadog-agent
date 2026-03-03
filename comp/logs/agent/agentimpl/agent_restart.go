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
	logsmetrics "github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

const (
	// Transport types for telemetry
	transportTCP  = "tcp"
	transportHTTP = "http"

	// Restart status for telemetry
	restartStatusSuccess = "success"
	restartStatusFailure = "failure"
)

// restart conducts a partial restart of the logs-agent pipeline with the provided endpoints.
// This is used to switch between transport protocols without disrupting the entire agent.
func (a *logAgent) restart(_ context.Context, newEndpoints *config.Endpoints) error {
	a.log.Info("Attempting to restart logs-agent pipeline")

	a.restartMutex.Lock()
	defer a.restartMutex.Unlock()

	// Store current endpoints for rollback if restart fails
	previousEndpoints := a.endpoints

	// Determine transport type for metrics
	targetTransport := transportTCP
	if newEndpoints.UseHTTP {
		targetTransport = transportHTTP
	}

	a.log.Info("Gracefully stopping logs-agent")

	timeout := a.config.GetDuration("logs_config.stop_grace_period")
	_, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := a.partialStop(); err != nil {
		a.log.Warn("Graceful partial stop timed out, force closing")
	}

	a.log.Info("Re-starting logs-agent...")

	a.endpoints = newEndpoints

	err := a.setupAgentForRestart()
	if err != nil {
		message := fmt.Sprintf("Could not re-start logs-agent: %v", err)
		a.log.Error(message)
		a.log.Error("Attempting rollback to previous transport")
		logsmetrics.TlmRestartAttempt.Inc(restartStatusFailure, targetTransport)
		return a.rollbackToPreviousTransport(previousEndpoints)
	}

	a.restartPipeline()
	logsmetrics.TlmRestartAttempt.Inc(restartStatusSuccess, targetTransport)
	return nil
}

// restartWithHTTPUpgrade upgrades the logs-agent pipeline to HTTP transport.
// This is called by the smart HTTP restart mechanism after HTTP connectivity has been verified.
//
// Since HTTP connectivity was verified before calling this function, we commit to HTTP
// and will keep retrying HTTP even if the upgrade fails. If restart fails, the base
// restart() function will rollback to TCP temporarily, but this function returns an
// error to trigger retry - ensuring we eventually upgrade to HTTP since connectivity exists.
func (a *logAgent) restartWithHTTPUpgrade(ctx context.Context) error {
	// Build HTTP endpoints since we already verified HTTP connectivity
	endpoints, err := buildHTTPEndpointsForRestart(a.config)
	if err != nil {
		message := fmt.Sprintf("Failed to build HTTP endpoints: %v", err)
		status.AddGlobalError(invalidEndpoints, message)
		a.log.Error(message)
		logsmetrics.TlmRestartAttempt.Inc(restartStatusFailure, transportHTTP)
		return errors.New(message)
	}

	err = a.restart(ctx, endpoints)
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
	status.Init(a.started, a.endpoints, a.sources, a.tracker, logsmetrics.LogsExpvars)

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

// smartHTTPRestart initiates periodic HTTP connectivity checks with exponential backoff
// to automatically upgrade from TCP to HTTP when connectivity is restored.
// This only runs when TCP fallback occurred (not when [force_]use_tcp is configured).
func (a *logAgent) smartHTTPRestart() {
	// Check if we're eligible for HTTP retry
	if config.ShouldUseTCP(a.config) {
		return
	}

	a.httpRetryMutex.Lock()
	// Cancel any existing loop to avoid leaks or duplicate retries
	if a.httpRetryCancel != nil {
		a.httpRetryCancel()
	}
	a.httpRetryCtx, a.httpRetryCancel = context.WithCancel(context.Background())
	ctx := a.httpRetryCtx
	a.httpRetryMutex.Unlock()

	a.log.Info("Starting HTTP connectivity retry with exponential backoff")

	// Start background goroutine for periodic HTTP checks
	go a.httpRetryLoop(ctx)
}

// httpRetryLoop runs periodic HTTP connectivity checks with exponential backoff
// Uses a similar backoff strategy as the TCP connection manager:
// exponential backoff with randomization [2^(n-1), 2^n) seconds, capped at configured max
func (a *logAgent) httpRetryLoop(ctx context.Context) {
	maxRetryInterval := config.HTTPConnectivityRetryIntervalMax(a.config)
	if maxRetryInterval.Seconds() <= 0 {
		a.log.Warn("HTTP connectivity retry interval max set to 0 seconds, skipping HTTP connectivity retry")
		return
	}

	endpoints, err := buildHTTPEndpointsForConnectivityCheck(a.config)
	if err != nil {
		a.log.Errorf("Failed to build HTTP endpoints: %v", err)
		return
	}

	policy := backoff.NewExpBackoffPolicy(
		endpoints.Main.BackoffFactor,
		endpoints.Main.BackoffBase,
		maxRetryInterval.Seconds(),
		endpoints.Main.RecoveryInterval,
		endpoints.Main.RecoveryReset,
	)

	attempt := 0
	for {
		// Calculate backoff interval similar to connection_manager.go
		backoffDuration := policy.GetBackoffDuration(attempt)

		a.log.Debugf("Next HTTP connectivity check in %v (attempt %d)", backoffDuration, attempt+1)

		select {
		case <-time.After(backoffDuration):
			attempt++
			a.log.Infof("Checking HTTP connectivity (attempt %d)", attempt)

			if a.checkHTTPConnectivity() {
				a.log.Info("HTTP connectivity restored - initiating upgrade to HTTP transport")

				// Trigger HTTP upgrade. Since HTTP connectivity is verified,
				// we commit to HTTP and keep retrying if upgrade fails.
				if err := a.restartWithHTTPUpgrade(ctx); err != nil {
					a.log.Errorf("HTTP upgrade failed: %v - will retry", err)
					// Publish retry failure metric
					logsmetrics.TlmHTTPConnectivityRetryAttempt.Inc("failure")
					// Continue retrying - HTTP is available, we want to use it
					continue
				}

				// Publish retry success metric
				logsmetrics.TlmHTTPConnectivityRetryAttempt.Inc("success")
				a.log.Info("Successfully upgraded to HTTP transport")
				return
			}

			a.log.Debug("HTTP connectivity check failed - will retry")

		case <-ctx.Done():
			a.log.Debug("HTTP retry loop stopped")
			return
		}
	}
}

// checkHTTPConnectivity tests if HTTP endpoints are reachable
func (a *logAgent) checkHTTPConnectivity() bool {
	endpoints, err := buildHTTPEndpointsForConnectivityCheck(a.config)
	if err != nil {
		a.log.Debugf("Failed to build HTTP endpoints for connectivity check: %v", err)
		return false
	}

	connectivity := checkHTTPConnectivityStatus(endpoints.Main, a.config)
	return connectivity == config.HTTPConnectivitySuccess
}

// stopHTTPRetry stops the HTTP retry loop
func (a *logAgent) stopHTTPRetry() {
	a.httpRetryMutex.Lock()
	defer a.httpRetryMutex.Unlock()

	if a.httpRetryCancel != nil {
		a.httpRetryCancel()
		a.httpRetryCancel = nil
		a.httpRetryCtx = nil
	}
}
