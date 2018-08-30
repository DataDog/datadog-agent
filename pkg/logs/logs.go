// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

var (
	// isRunning indicates whether logs-agent is running or not
	isRunning bool
	// logs-agent
	agent *Agent
	// scheduler is plugged to autodiscovery to collect integration configs and schedule log collection for different kind of inputs
	scheduler *Scheduler
)

// Start starts logs-agent
func Start() error {
	// setup the server config
	serverConfig, err := config.BuildServerConfig()
	if err != nil {
		return err
	}

	// setup the log-sources
	sources := config.NewLogSources()
	for _, source := range config.DefaultSources() {
		sources.AddSource(source)
	}

	// setup the services
	services := service.NewServices()

	// initialize the config scheduler
	scheduler = NewScheduler(sources, services)

	// setup the status
	status.Initialize(sources)

	// setup and start the agent
	agent = NewAgent(sources, services, serverConfig)
	log.Info("Starting logs-agent")
	agent.Start()
	isRunning = true

	return nil
}

// Stop stops properly the logs-agent to prevent data loss,
// it only returns when the whole pipeline is flushed.
func Stop() {
	if isRunning {
		log.Info("Stopping logs-agent")
		agent.Stop()
	}
}

// IsAgentRunning returns true if the logs-agent is running.
func IsAgentRunning() bool {
	return isRunning
}

// GetStatus returns logs-agent status
func GetStatus() status.Status {
	if !isRunning {
		return status.Status{IsRunning: false}
	}
	return status.Get()
}

// GetScheduler returns the logs-config scheduler if set.
func GetScheduler() *Scheduler {
	return scheduler
}
