// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traps

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
)

// Forwarder forwards trap packets as messages to the logs pipeline.
type Forwarder struct {
	serverOutput     traps.OutputChannel
	source           *config.LogSource
	pipelineProvider pipeline.Provider
	tailer           *Tailer
}

// NewForwarder returns a new forwarder.
func NewForwarder(serverOutput traps.OutputChannel, source *config.LogSource, pipelineProvider pipeline.Provider) *Forwarder {
	return &Forwarder{
		serverOutput:     serverOutput,
		source:           source,
		pipelineProvider: pipelineProvider,
	}
}

// Start starts the forwarder.
func (f *Forwarder) Start() {
	f.startNewTailer()
	f.source.Status.Success()
}

func (f *Forwarder) startNewTailer() {
	inputChan := f.serverOutput
	outputChan := f.pipelineProvider.NextPipelineChan()
	f.tailer = NewTailer(f.source, inputChan, outputChan)
	f.tailer.Start()
}

// Stop stops the forwarder.
func (f *Forwarder) Stop() {
	if f.tailer != nil {
		f.tailer.Stop()
	}
}
