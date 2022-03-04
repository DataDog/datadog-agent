// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	metaScheduler "github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	adScheduler "github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"
	ccaScheduler "github.com/DataDog/datadog-agent/pkg/logs/schedulers/cca"
	trapsScheduler "github.com/DataDog/datadog-agent/pkg/logs/schedulers/traps"
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
)

var (
	// isRunning indicates whether logs-agent is running or not
	isRunning int32
	// logs-agent
	agent *Agent
)

// Start starts logs-agent
// getAC is a func returning the prepared AutoConfig. It is nil until
// the AutoConfig is ready, please consider using BlockUntilAutoConfigRanOnce
// instead of directly using it.
// The parameter serverless indicates whether or not this Logs Agent is running
// in a serverless environment.
func Start(getAC func() *autodiscovery.AutoConfig) (*Agent, error) {
	return start(getAC, false)
}

// StartServerless starts a Serverless instance of the Logs Agent.
func StartServerless(getAC func() *autodiscovery.AutoConfig) (*Agent, error) {
	return start(getAC, true)
}

// buildEndpoints builds endpoints for the logs agent
func buildEndpoints(serverless bool) (*config.Endpoints, error) {
	if serverless {
		return config.BuildServerlessEndpoints(intakeTrackType, config.DefaultIntakeProtocol)
	}
	httpConnectivity := config.HTTPConnectivityFailure
	if endpoints, err := config.BuildHTTPEndpoints(intakeTrackType, AgentJSONIntakeProtocol, config.DefaultIntakeOrigin); err == nil {
		httpConnectivity = http.CheckConnectivity(endpoints.Main)
	}
	return config.BuildEndpoints(httpConnectivity, intakeTrackType, AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
}

func start(getAC func() *autodiscovery.AutoConfig, serverless bool) (*Agent, error) {
	if IsAgentRunning() {
		return agent, nil
	}

	// setup the sources and the services
	sources := config.NewLogSources()
	services := service.NewServices()

	// setup the server config
	endpoints, err := buildEndpoints(serverless)

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
	status.Init(&isRunning, endpoints, sources, metrics.LogsExpvars)

	// setup global processing rules
	processingRules, err := config.GlobalProcessingRules()
	if err != nil {
		message := fmt.Sprintf("Invalid processing rules: %v", err)
		status.AddGlobalError(invalidProcessingRules, message)
		return nil, errors.New(message)
	}

	// setup and start the logs agent
	if !serverless {
		// regular logs agent
		log.Info("Starting logs-agent...")
		agent = NewAgent(sources, services, processingRules, endpoints)
	} else {
		// serverless logs agent
		log.Info("Starting a serverless logs-agent...")
		agent = NewServerless(sources, services, processingRules, endpoints)
	}

	agent.Start()
	atomic.StoreInt32(&isRunning, 1)
	log.Info("logs-agent started")

	agent.AddScheduler(adScheduler.New())
	agent.AddScheduler(ccaScheduler.New(getAC()))
	agent.AddScheduler(trapsScheduler.New())

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
		atomic.StoreInt32(&isRunning, 0)
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

// SetADMetaScheduler provides a reference to the AD MetaScheduler during agent
// startup.
func SetADMetaScheduler(sched *metaScheduler.MetaScheduler) {
	adScheduler.ADMetaScheduler = sched
}

// GetMessageReceiver returns the diagnostic message receiver
func GetMessageReceiver() *diagnostic.BufferedMessageReceiver {
	if agent == nil {
		return nil
	}
	return agent.diagnosticMessageReceiver
}
