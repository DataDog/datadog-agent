// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"github.com/StackVista/stackstate-agent/pkg/logs/auditor"
	"github.com/StackVista/stackstate-agent/pkg/logs/config"
	"github.com/StackVista/stackstate-agent/pkg/logs/input/container"
	"github.com/StackVista/stackstate-agent/pkg/logs/input/file"
	"github.com/StackVista/stackstate-agent/pkg/logs/input/journald"
	"github.com/StackVista/stackstate-agent/pkg/logs/input/listener"
	"github.com/StackVista/stackstate-agent/pkg/logs/input/windowsevent"
	"github.com/StackVista/stackstate-agent/pkg/logs/message"
	"github.com/StackVista/stackstate-agent/pkg/logs/pipeline"
	"github.com/StackVista/stackstate-agent/pkg/logs/restart"
	"github.com/StackVista/stackstate-agent/pkg/logs/sender"
	"github.com/StackVista/stackstate-agent/pkg/logs/service"
)

// Agent represents the data pipeline that collects, decodes,
// processes and sends logs to the backend
// + ------------------------------------------------------ +
// |                                                        |
// | Collector -> Decoder -> Processor -> Sender -> Auditor |
// |                                                        |
// + ------------------------------------------------------ +
type Agent struct {
	auditor          *auditor.Auditor
	pipelineProvider pipeline.Provider
	inputs           []restart.Restartable
}

// NewAgent returns a new Agent
func NewAgent(sources *config.LogSources, services *service.Services, serverConfig *config.ServerConfig) *Agent {
	// setup the auditor
	messageChan := make(chan message.Message, config.ChanSize)
	auditor := auditor.New(messageChan, config.LogsAgent.GetString("logs_config.run_path"))

	// setup the pipeline provider that provides pairs of processor and sender
	connectionManager := sender.NewConnectionManager(serverConfig, config.LogsAgent.GetString("logs_config.socks5_proxy_address"))
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, connectionManager, messageChan)

	// setup the inputs
	inputs := []restart.Restartable{
		container.NewLauncher(sources, services, pipelineProvider, auditor),
		listener.NewListener(sources, config.LogsAgent.GetInt("logs_config.frame_size"), pipelineProvider),
		file.NewScanner(sources, config.LogsAgent.GetInt("logs_config.open_files_limit"), pipelineProvider, auditor, file.DefaultSleepDuration),
		journald.NewLauncher(sources, pipelineProvider, auditor),
		windowsevent.NewLauncher(sources, pipelineProvider),
	}

	return &Agent{
		auditor:          auditor,
		pipelineProvider: pipelineProvider,
		inputs:           inputs,
	}
}

// Start starts all the elements of the data pipeline
// in the right order to prevent data loss
func (a *Agent) Start() {
	starter := restart.NewStarter(a.auditor, a.pipelineProvider)
	for _, input := range a.inputs {
		starter.Add(input)
	}
	starter.Start()
}

// Stop stops all the elements of the data pipeline
// in the right order to prevent data loss
func (a *Agent) Stop() {
	inputs := restart.NewParallelStopper()
	for _, input := range a.inputs {
		inputs.Add(input)
	}
	stopper := restart.NewSerialStopper(
		inputs,
		a.pipelineProvider,
		a.auditor,
	)
	stopper.Stop()
}
