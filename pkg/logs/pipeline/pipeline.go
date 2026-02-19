// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pipeline provides log processing pipeline functionality
package pipeline

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

// Pipeline processes and sends messages to the backend
type Pipeline struct {
	InputChan       chan *message.Message
	flushChan       chan struct{}
	processor       *processor.Processor
	strategy        sender.Strategy
	pipelineMonitor metrics.PipelineMonitor
}

// NewPipeline returns a new Pipeline
func NewPipeline(
	processingRules []*config.ProcessingRule,
	endpoints *config.Endpoints,
	senderImpl sender.PipelineComponent,
	diagnosticMessageReceiver diagnostic.MessageReceiver,
	serverlessMeta sender.ServerlessMeta,
	hostname hostnameinterface.Component,
	cfg pkgconfigmodel.Reader,
	compressorPool *CompressorPool,
	compressorIndex int,
	instanceID string,
) *Pipeline {
	strategyInput := make(chan *message.Message, pkgconfigsetup.Datadog().GetInt("logs_config.message_channel_size"))
	flushChan := make(chan struct{})

	var encoder processor.Encoder
	if serverlessMeta.IsEnabled() {
		encoder = processor.JSONServerlessInitEncoder
	} else if endpoints.UseHTTP {
		encoder = processor.JSONEncoder
	} else if endpoints.UseProto {
		encoder = processor.ProtoEncoder
	} else {
		encoder = processor.RawEncoder
	}
	strategy := getStrategy(strategyInput, senderImpl.In(), flushChan, endpoints, serverlessMeta, senderImpl.PipelineMonitor(), compressorPool, compressorIndex, instanceID)

	inputChan := make(chan *message.Message, pkgconfigsetup.Datadog().GetInt("logs_config.message_channel_size"))

	processor := processor.New(cfg, inputChan, strategyInput, processingRules,
		encoder, diagnosticMessageReceiver, hostname, senderImpl.PipelineMonitor(), instanceID)

	return &Pipeline{
		InputChan:       inputChan,
		flushChan:       flushChan,
		processor:       processor,
		strategy:        strategy,
		pipelineMonitor: senderImpl.PipelineMonitor(),
	}
}

// Start launches the pipeline
func (p *Pipeline) Start() {
	p.strategy.Start()
	p.processor.Start()
}

// Stop stops the pipeline
func (p *Pipeline) Stop() {
	p.processor.Stop()
	p.strategy.Stop()
}

// Flush flushes synchronously the processor and sender managed by this pipeline.
func (p *Pipeline) Flush(ctx context.Context) {
	p.flushChan <- struct{}{}
	p.processor.Flush(ctx) // flush messages in the processor into the sender
}

func getStrategy(
	inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	endpoints *config.Endpoints,
	serverlessMeta sender.ServerlessMeta,
	pipelineMonitor metrics.PipelineMonitor,
	compressorPool *CompressorPool,
	compressorIndex int,
	instanceID string,
) sender.Strategy {
	compressor := compressorPool.GetCompressor(compressorIndex)

	if endpoints.UseHTTP || serverlessMeta.IsEnabled() {
		return sender.NewBatchStrategy(
			inputChan,
			outputChan,
			flushChan,
			serverlessMeta,
			endpoints.BatchWait,
			endpoints.BatchMaxSize,
			endpoints.BatchMaxContentSize,
			"logs",
			compressor,
			pipelineMonitor,
			instanceID)
	}
	return sender.NewStreamStrategy(inputChan, outputChan, compressor)
}
