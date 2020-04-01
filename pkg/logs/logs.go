// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package logs

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/metrics"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/scheduler"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

const (
	// key used to display a warning message on the agent status
	invalidProcessingRules = "invalid_global_processing_rules"
	invalidEndpoints       = "invalid_endpoints"
)

// Transport is the transport used by logs-agent, i.e TCP or HTTP
type Transport string

const (
	// TransportHTTP indicates logs-agent is using HTTP transport
	TransportHTTP Transport = "HTTP"
	// TransportTCP indicates logs-agent is using TCP transport
	TransportTCP Transport = "TCP"
)

var (
	// isRunning indicates whether logs-agent is running or not
	isRunning int32
	// logs-agent
	agent *Agent
	// scheduler is plugged to autodiscovery to collect integration configs
	// and schedule log collection for different kind of inputs
	adScheduler *scheduler.Scheduler
	// CurrentTransport is the current transport used by logs-agent, i.e TCP or HTTP
	CurrentTransport Transport
)

// Start starts logs-agent
func Start() error {
	if IsAgentRunning() {
		return nil
	}

	// setup the sources and the services
	sources := config.NewLogSources()
	services := service.NewServices()

	// setup the config scheduler
	adScheduler = scheduler.NewScheduler(sources, services)

	// setup the server config
	httpConnectivity := config.HTTPConnectivityFailure
	if endpoints, err := config.BuildHTTPEndpoints(); err == nil {
		httpConnectivity = http.CheckConnectivity(endpoints.Main)
	}
	endpoints, err := config.BuildEndpoints(httpConnectivity)
	if err != nil {
		message := fmt.Sprintf("Invalid endpoints: %v", err)
		status.AddGlobalError(invalidEndpoints, message)
		return errors.New(message)
	}
	CurrentTransport = TransportTCP
	if endpoints.UseHTTP {
		CurrentTransport = TransportHTTP
	}

	// setup the status
	status.Init(&isRunning, endpoints, sources, metrics.LogsExpvars)

	// setup global processing rules
	processingRules, err := config.GlobalProcessingRules()
	if err != nil {
		message := fmt.Sprintf("Invalid processing rules: %v", err)
		status.AddGlobalError(invalidProcessingRules, message)
		return errors.New(message)
	}

	// setup and start the agent
	agent = NewAgent(sources, services, processingRules, endpoints)
	log.Info("Starting logs-agent...")
	agent.Start()
	atomic.StoreInt32(&isRunning, 1)
	log.Info("logs-agent started")

	// add the default sources
	for _, source := range config.DefaultSources() {
		sources.AddSource(source)
	}

	return nil
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
		if adScheduler != nil {
			adScheduler.Stop()
			adScheduler = nil
		}
		status.Clear()
		atomic.StoreInt32(&isRunning, 0)
	}
	log.Info("logs-agent stopped")
}

// IsAgentRunning returns true if the logs-agent is running.
func IsAgentRunning() bool {
	return status.Get().IsRunning
}

// GetStatus returns logs-agent status
func GetStatus() status.Status {
	return status.Get()
}

// GetScheduler returns the logs-config scheduler if set.
func GetScheduler() *scheduler.Scheduler {
	return adScheduler
}
