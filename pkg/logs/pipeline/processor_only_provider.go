// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
)

// processorOnlyProvider implements the Provider provider interface and only contains the processor
type processorOnlyProvider struct {
	processor       *processor.Processor
	inputChan       chan *message.Message
	outputChan      chan *message.Message
	pipelineMonitor metrics.PipelineMonitor
}

// NewProcessorOnlyProvider is used by the logs check subcommand as the feature does not require the functionalities of the log pipeline other then the processor.
func NewProcessorOnlyProvider(diagnosticMessageReceiver diagnostic.MessageReceiver, processingRules []*config.ProcessingRule, hostname hostnameinterface.Component) Provider {
	chanSize := pkgconfigsetup.Datadog().GetInt("logs_config.message_channel_size")
	outputChan := make(chan *message.Message, chanSize)
	encoder := processor.JSONEncoder
	inputChan := make(chan *message.Message, chanSize)
	pipelineID := "0"
	pipelineMonitor := metrics.NewNoopPipelineMonitor(pipelineID)
	processor := processor.New(nil, inputChan, outputChan, processingRules,
		encoder, diagnosticMessageReceiver, hostname, pipelineMonitor, pipelineID)

	p := &processorOnlyProvider{
		processor:       processor,
		inputChan:       inputChan,
		outputChan:      outputChan,
		pipelineMonitor: pipelineMonitor,
	}

	return p
}

func (p *processorOnlyProvider) Start() {
	p.processor.Start()
}

func (p *processorOnlyProvider) Stop() {
	p.processor.Stop()
}

func (p *processorOnlyProvider) NextPipelineChan() chan *message.Message {
	return p.inputChan
}

func (p *processorOnlyProvider) NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor) {
	return p.inputChan, p.pipelineMonitor.GetCapacityMonitor(metrics.ProcessorTlmName, "0")
}

func (p *processorOnlyProvider) GetOutputChan() chan *message.Message {
	return p.outputChan
}

// Flush flushes synchronously all the contained pipeline of this provider.
func (p *processorOnlyProvider) Flush(ctx context.Context) {
	p.processor.Flush(ctx)
}
