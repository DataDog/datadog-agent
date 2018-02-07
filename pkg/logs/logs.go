// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input/container"
	"github.com/DataDog/datadog-agent/pkg/logs/input/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/input/tailer"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

var (
	// isRunning indicates whether logs-agent is running or not
	isRunning bool
	// logs-agent data pipeline
	agentPipeline restart.Stopper
)

// Start starts logs-agent
func Start() error {
	sources, err := config.Build()
	if err != nil {
		return err
	}
	go run(sources)
	return nil
}

// run sets up the pipeline to process logs and send them to Datadog back-end
func run(sources *config.LogSources) {
	connectionManager := sender.NewConnectionManager(
		config.LogsAgent.GetString("logs_config.dd_url"),
		config.LogsAgent.GetInt("logs_config.dd_port"),
		config.LogsAgent.GetBool("logs_config.dev_mode_no_ssl"),
	)

	messageChan := make(chan message.Message, config.ChanSize)
	auditor := auditor.New(messageChan, config.LogsAgent.GetString("logs_config.run_path"))

	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, connectionManager, messageChan)

	networkListeners := listener.New(sources.GetValidSources(), pipelineProvider)
	containersScanner := container.New(sources.GetValidSources(), pipelineProvider, auditor)
	filesScanner := tailer.New(sources.GetValidSources(), config.LogsAgent.GetInt("logs_config.open_files_limit"), pipelineProvider, auditor, tailer.DefaultSleepDuration)

	restart.Start(auditor, pipelineProvider, networkListeners, containersScanner, filesScanner)
	status.Initialize(sources.GetSources())

	inputs := restart.NewParallelStopper(filesScanner, containersScanner, networkListeners)
	agentPipeline = restart.NewSerialStopper(inputs, pipelineProvider, auditor)

	isRunning = true
}

// Stop stops properly the logs-agent to prevent data loss
// All Stop methods are blocking which means that Stop only returns
// when the whole pipeline is flushed
func Stop() {
	if isRunning {
		log.Info("Stopping logs-agent")
		agentPipeline.Stop()
	}
}

// GetStatus returns logs-agent status
func GetStatus() status.Status {
	if !isRunning {
		return status.Status{IsRunning: false}
	}
	return status.Get()
}
