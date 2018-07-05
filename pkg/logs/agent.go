// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input/container"
	"github.com/DataDog/datadog-agent/pkg/logs/input/file"
	"github.com/DataDog/datadog-agent/pkg/logs/input/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/input/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/input/windowsevent"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	scanners         []interface{}
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

	// setup the scanners
	validSources := sources.GetValidSources()
	var scanners []interface{}
	scanners = append(scanners, listener.New(validSources, pipelineProvider))
	scanners = append(scanners, file.New(sources, config.LogsAgent.GetInt("logs_config.open_files_limit"), pipelineProvider, auditor, file.DefaultSleepDuration))
	scanners = append(scanners, journald.New(validSources, pipelineProvider, auditor))
	scanners = append(scanners, windowsevent.New(sources, pipelineProvider, auditor))
	scanners = append(scanners, container.NewScanner(sources, pipelineProvider, auditor))

	return &Agent{
		auditor:          auditor,
		pipelineProvider: pipelineProvider,
		scanners:         scanners,
	}
}

// Start starts all the elements of the data pipeline
// in the right order to prevent data loss
func (a *Agent) Start() {
	restart.Start(a.auditor, a.pipelineProvider)
	for _, scanner := range a.scanners {
		if start, ok := scanner.(restart.Startable); ok {
			start.Start()
		} else {
			log.Errorf("error starting scanner %s: does not implement Startable", scanner)
		}
	}
}

// Stop stops all the elements of the data pipeline
// in the right order to prevent data loss
func (a *Agent) Stop() {
	scanners := restart.NewParallelStopper()
	for _, scanner := range a.scanners {
		if stop, ok := scanner.(restart.Stoppable); ok {
			scanners.Add(stop)
		} else {
			log.Errorf("error stopping scanner %s: does not implement Stoppable", scanner)
		}
	}
	stopper := restart.NewSerialStopper(
		scanners,
		a.pipelineProvider,
		a.auditor,
	)
	stopper.Stop()
}
