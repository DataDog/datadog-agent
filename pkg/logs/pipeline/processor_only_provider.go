// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
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

// NewProcessorOnlyProvider creates a provider with the JSON encoder, suitable
// for the analyze-logs subcommand which reads JSON from GetOutputChan().
func NewProcessorOnlyProvider(diagnosticMessageReceiver diagnostic.MessageReceiver, processingRules []*config.ProcessingRule, hostname hostnameinterface.Component, cfg pkgconfigmodel.Reader) Provider {
	return NewProcessorOnlyProviderWithEncoder(diagnosticMessageReceiver, processingRules, hostname, cfg, processor.JSONEncoder)
}

// NewProcessorOnlyProviderWithEncoder creates a provider that uses the supplied
// encoder. Use processor.PassthroughEncoder when the consumer reads
// GetContent() directly (e.g. the observer) to avoid JSON-wrapping the content.
func NewProcessorOnlyProviderWithEncoder(diagnosticMessageReceiver diagnostic.MessageReceiver, processingRules []*config.ProcessingRule, hostname hostnameinterface.Component, cfg pkgconfigmodel.Reader, enc processor.Encoder) Provider {
	chanSize := cfg.GetInt("logs_config.message_channel_size")
	outputChan := make(chan *message.Message, chanSize)
	inputChan := make(chan *message.Message, chanSize)
	pipelineID := "0"
	pipelineMonitor := metrics.NewNoopPipelineMonitor(pipelineID)
	proc := processor.New(nil, inputChan, outputChan, processingRules,
		enc, diagnosticMessageReceiver, hostname, pipelineMonitor, pipelineID)

	p := &processorOnlyProvider{
		processor:       proc,
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
