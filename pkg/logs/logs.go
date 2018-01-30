// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	log "github.com/cihub/seelog"
	"github.com/spf13/viper"

	aud "github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input"
	"github.com/DataDog/datadog-agent/pkg/logs/input/container"
	"github.com/DataDog/datadog-agent/pkg/logs/input/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/input/tailer"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

var (
	// isRunning indicates whether logs-agent is running or not
	isRunning bool

	// input components
	inputs *input.Inputs

	// pipeline provider
	pipelineProvider pipeline.Provider

	// auditor
	auditor *aud.Auditor
)

// Start starts logs-agent
func Start(ddConfig *viper.Viper) error {
	config, err := config.Build(ddConfig)
	if err != nil {
		return err
	}
	go run(config)
	return nil
}

// run sets up the pipeline to process logs and send them to Datadog back-end
func run(config *config.Config) {
	isRunning = true

	connectionManager := sender.NewConnectionManager(config)

	messageChan := make(chan message.Message, config.GetChanSize())
	auditor := auditor.New(messageChan, config.GetRunPath())
	auditor.Start()

	pipelineProvider := pipeline.NewProvider(config)
	pipelineProvider.Start(connectionManager, messageChan)

	sources := config.GetLogsSources()

	networkListeners := listener.New(sources.GetValidSources(), pipelineProvider)
	containersScanner := container.New(sources.GetValidSources(), pipelineProvider, auditor)
	filesScanner = tailer.New(sources.GetValidSources(), config.GetOpenFilesLimit(), pipelineProvider, auditor, tailer.DefaultSleepDuration)

	inputs = input.NewInputs([]input.Input{
		networkListeners,
		filesScanner,
		containersScanner,
	})
	inputs.Start()

	status.Initialize(sources.GetSources())
}

// Stop stops properly the logs-agent to prevent data loss
// All Stop methods are blocking which means that Stop only returns
// when the whole pipeline is flushed
func Stop() {
	log.Info("Stopping logs-agent")
	if isRunning {
		// stop all input components
		inputs.Stop()

		// stop all the different pipelines
		pipelineProvider.Stop()

		// stop the auditor
		auditor.Stop()
	}
}

// GetStatus returns logs-agent status
func GetStatus() status.Status {
	if !isRunning {
		return status.Status{IsRunning: false}
	}
	return status.Get()
}
