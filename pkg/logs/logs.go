// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

var (
	// isRunning indicates whether logs-agent is running or not
	isRunning bool
	// logs-agent
	agent *Agent
)

// Start starts logs-agent
func Start() error {
	// build log sources
	sources, err := config.Build()
	if err != nil {
		// could not find any valid logs configuration
		return err
	}

	// setup and start the agent
	log.Info("Starting logs-agent")
	agent = NewAgent(sources)
	agent.Start()

	// setup the status
	status.Initialize(sources)

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

// GetStatus returns logs-agent status
func GetStatus() status.Status {
	if !isRunning {
		return status.Status{IsRunning: false}
	}
	return status.Get()
}
