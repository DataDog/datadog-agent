// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package logs

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/scheduler"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

var (
	// isRunning indicates whether logs-agent is running or not
	isRunning bool
	// logs-agent
	agent *Agent
	// scheduler is plugged to autodiscovery to collect integration configs
	// and schedule log collection for different kind of inputs
	adScheduler *scheduler.Scheduler
)

// Start starts logs-agent
func Start() error {
	if IsAgentRunning() {
		return nil
	}
	// setup the server config
	endpoints, err := config.BuildEndpoints()
	if err != nil {
		return err
	}

	// setup the sources and the services
	sources := config.NewLogSources()
	services := service.NewServices()

	// initialize the config scheduler
	adScheduler = scheduler.NewScheduler(sources, services)

	// setup the status
	status.Initialize(sources)

	// setup and start the agent
	agent = NewAgent(sources, services, endpoints)
	log.Info("Starting logs-agent...")
	agent.Start()
	isRunning = true
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
	if isRunning {
		if agent != nil {
			agent.Stop()
			agent = nil
		}
		if adScheduler != nil {
			adScheduler.Stop()
			adScheduler = nil
		}
		status.Clear()
		isRunning = false
	}
	log.Info("logs-agent stopped")
}

// IsAgentRunning returns true if the logs-agent is running.
func IsAgentRunning() bool {
	return isRunning
}

// GetStatus returns logs-agent status
func GetStatus() status.Status {
	if !IsAgentRunning() {
		return status.Status{IsRunning: false}
	}
	return status.Get()
}

// GetScheduler returns the logs-config scheduler if set.
func GetScheduler() *scheduler.Scheduler {
	return adScheduler
}
