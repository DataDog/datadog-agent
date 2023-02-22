// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

const (
	// key used to display a warning message on the agent status
	invalidProcessingRules = "invalid_global_processing_rules"
	invalidEndpoints       = "invalid_endpoints"
	intakeTrackType        = "logs"

	// AgentJSONIntakeProtocol agent json protocol
	AgentJSONIntakeProtocol = "agent-json"

	// Log messages
	multiLineWarning = "multi_line processing rules are not supported as global processing rules."
)

var (
	// isRunning indicates whether logs-agent is running or not
	isRunning = atomic.NewBool(false)
	// logs-agent
	agent *Agent
)

// StartServerless starts a Serverless instance of the Logs Agent.
func StartServerless() (*Agent, error) {
	return start()
}

func start() (*Agent, error) {
	if IsAgentRunning() {
		return agent, nil
	}

	// setup the sources and the services
	sources := sources.NewLogSources()
	services := service.NewServices()

	// setup the server config
	endpoints, err := buildEndpoints()

	if err != nil {
		message := fmt.Sprintf("Invalid endpoints: %v", err)
		status.AddGlobalError(invalidEndpoints, message)
		return nil, errors.New(message)
	}
	status.CurrentTransport = status.TransportTCP
	if endpoints.UseHTTP {
		status.CurrentTransport = status.TransportHTTP
	}
	inventories.SetAgentMetadata(inventories.AgentLogsTransport, status.CurrentTransport)

	// setup the status
	status.Init(isRunning, endpoints, sources, metrics.LogsExpvars)

	// setup global processing rules
	processingRules, err := config.GlobalProcessingRules()
	if err != nil {
		message := fmt.Sprintf("Invalid processing rules: %v", err)
		status.AddGlobalError(invalidProcessingRules, message)
		return nil, errors.New(message)
	}

	if config.HasMultiLineRule(processingRules) {
		log.Warn(multiLineWarning)
		status.AddGlobalWarning(invalidProcessingRules, multiLineWarning)
	}

	// setup and start the logs agent
	log.Info("Starting logs-agent...")
	agent = NewAgent(sources, services, processingRules, endpoints)

	agent.Start()
	isRunning.Store(true)
	log.Info("logs-agent started")

	return agent, nil
}

// Stop stops properly the logs-agent to prevent data loss,
// it only returns when the whole pipeline is flushed.
func Stop() {
	log.Info("Stopping logs-agent")
	if IsAgentRunning() {
		if agent != nil {
			agent.Stop()
			agent = nil
		}
		status.Clear()
		isRunning.Store(false)
	}
	log.Info("logs-agent stopped")
}

// Flush flushes synchronously the running instance of the Logs Agent.
// Use a WithTimeout context in order to have a flush that can be cancelled.
func Flush(ctx context.Context) {
	log.Info("Triggering a flush in the logs-agent")
	if IsAgentRunning() {
		if agent != nil {
			agent.Flush(ctx)
		}
	}
	log.Debug("Flush in the logs-agent done.")
}

// IsAgentRunning returns true if the logs-agent is running.
func IsAgentRunning() bool {
	return status.Get().IsRunning
}

// GetStatus returns logs-agent status
func GetStatus() status.Status {
	return status.Get()
}

// GetMessageReceiver returns the diagnostic message receiver
func GetMessageReceiver() *diagnostic.BufferedMessageReceiver {
	if agent == nil {
		return nil
	}
	return agent.diagnosticMessageReceiver
}
