// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input/container"
	"github.com/DataDog/datadog-agent/pkg/logs/input/eventlog"
	"github.com/DataDog/datadog-agent/pkg/logs/input/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/input/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/input/tailer"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

// Agent represents the data pipeline that collects, decodes,
// processes and sends logs to the backend
// + ------------------------------------------------------ +
// |                                                        |
// | Collector -> Decoder -> Processor -> Sender -> Auditor |
// |                                                        |
// + ------------------------------------------------------ +
type Agent struct {
	auditor           *auditor.Auditor
	containersScanner *container.Scanner
	eventlogLauncher  *eventlog.Launcher
	filesScanner      *tailer.Scanner
	networkListener   *listener.Listener
	journaldLauncher  *journald.Launcher
	pipelineProvider  pipeline.Provider
}

// NewAgent returns a new Agent
func NewAgent(sources *config.LogSources) *Agent {
	// setup the auditor
	messageChan := make(chan message.Message, config.ChanSize)
	auditor := auditor.New(messageChan, config.LogsAgent.GetString("logs_config.run_path"))

	// setup the pipeline provider that provides pairs of processor and sender
	connectionManager := sender.NewConnectionManager(
		config.LogsAgent.GetString("logs_config.dd_url"),
		config.LogsAgent.GetInt("logs_config.dd_port"),
		config.LogsAgent.GetBool("logs_config.dev_mode_no_ssl"),
	)
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, connectionManager, messageChan)

	// setup the collectors
	validSources := sources.GetValidSources()
	containersScanner := container.New(validSources, pipelineProvider, auditor)
	networkListeners := listener.New(validSources, pipelineProvider)
	filesScanner := tailer.New(validSources, config.LogsAgent.GetInt("logs_config.open_files_limit"), pipelineProvider, auditor, tailer.DefaultSleepDuration)
	journaldLauncher := journald.New(validSources, pipelineProvider, auditor)
	eventlogLauncher := eventlog.New(validSources, pipelineProvider, auditor)

	return &Agent{
		auditor:           auditor,
		containersScanner: containersScanner,
		eventlogLauncher:  eventlogLauncher,
		filesScanner:      filesScanner,
		journaldLauncher:  journaldLauncher,
		networkListener:   networkListeners,
		pipelineProvider:  pipelineProvider,
	}
}

// Start starts all the elements of the data pipeline
// in the right order to prevent data loss
func (a *Agent) Start() {
	restart.Start(
		a.auditor,
		a.pipelineProvider,
		a.filesScanner,
		a.networkListener,
		a.containersScanner,
		a.journaldLauncher,
		a.eventlogLauncher,
	)
}

// Stop stops all the elements of the data pipeline
// in the right order to prevent data loss
func (a *Agent) Stop() {
	stopper := restart.NewSerialStopper(
		restart.NewParallelStopper(
			a.filesScanner,
			a.networkListener,
			a.containersScanner,
			a.journaldLauncher,
			a.eventlogLauncher,
		),
		a.pipelineProvider,
		a.auditor,
	)
	stopper.Stop()
}
