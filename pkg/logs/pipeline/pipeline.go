// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

// Pipeline processes and sends messages to the backend
type Pipeline struct {
	InputChan chan *message.Message
	flushChan chan struct{}
	processor *processor.Processor
	strategy  sender.Strategy
	sender    *sender.Sender
}

// NewPipeline returns a new Pipeline
func NewPipeline(outputChan chan *message.Payload,
	processingRules []*config.ProcessingRule,
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	diagnosticMessageReceiver diagnostic.MessageReceiver,
	serverless bool,
	pipelineID int) *Pipeline {

	mainDestinations := getDestinations(endpoints, destinationsContext, pipelineID, serverless)

	strategyInput := make(chan *message.Message, config.ChanSize)
	senderInput := make(chan *message.Payload, 1) // Only buffer 1 message since payloads can be large
	flushChan := make(chan struct{})

	var logsSender *sender.Sender

	var encoder processor.Encoder
	if serverless {
		encoder = processor.JSONServerlessEncoder
	} else if endpoints.UseHTTP {
		encoder = processor.JSONEncoder
	} else if endpoints.UseProto {
		encoder = processor.ProtoEncoder
	} else {
		encoder = processor.RawEncoder
	}

	strategy := getStrategy(strategyInput, senderInput, flushChan, endpoints, serverless, pipelineID)
	logsSender = sender.NewSender(senderInput, outputChan, mainDestinations, config.DestinationPayloadChanSize)

	inputChan := make(chan *message.Message, config.ChanSize)
	processor := processor.New(inputChan, strategyInput, processingRules, encoder, diagnosticMessageReceiver)

	return &Pipeline{
		InputChan: inputChan,
		flushChan: flushChan,
		processor: processor,
		strategy:  strategy,
		sender:    logsSender,
	}
}

// Start launches the pipeline
func (p *Pipeline) Start() {
	p.sender.Start()
	p.strategy.Start()
	p.processor.Start()
}

// Stop stops the pipeline
func (p *Pipeline) Stop() {
	p.processor.Stop()
	p.strategy.Stop()
	p.sender.Stop()
}

// Flush flushes synchronously the processor and sender managed by this pipeline.
func (p *Pipeline) Flush(ctx context.Context) {
	p.flushChan <- struct{}{}
	p.processor.Flush(ctx) // flush messages in the processor into the sender
}

func getDestinations(endpoints *config.Endpoints, destinationsContext *client.DestinationsContext, pipelineID int, serverless bool) *client.Destinations {
	reliable := []client.Destination{}
	additionals := []client.Destination{}

	if endpoints.UseHTTP {
		for i, endpoint := range endpoints.GetReliableEndpoints() {
			telemetryName := fmt.Sprintf("logs_%d_reliable_%d", pipelineID, i)
			reliable = append(reliable, http.NewDestination(endpoint, http.JSONContentType, destinationsContext, endpoints.BatchMaxConcurrentSend, !serverless, telemetryName))
		}
		for i, endpoint := range endpoints.GetUnReliableEndpoints() {
			telemetryName := fmt.Sprintf("logs_%d_unreliable_%d", pipelineID, i)
			additionals = append(additionals, http.NewDestination(endpoint, http.JSONContentType, destinationsContext, endpoints.BatchMaxConcurrentSend, false, telemetryName))
		}
		return client.NewDestinations(reliable, additionals)
	}
	for _, endpoint := range endpoints.GetReliableEndpoints() {
		reliable = append(reliable, tcp.NewDestination(endpoint, endpoints.UseProto, destinationsContext, !serverless))
	}
	for _, endpoint := range endpoints.GetUnReliableEndpoints() {
		additionals = append(additionals, tcp.NewDestination(endpoint, endpoints.UseProto, destinationsContext, false))
	}
	return client.NewDestinations(reliable, additionals)
}

func getStrategy(inputChan chan *message.Message, outputChan chan *message.Payload, flushChan chan struct{}, endpoints *config.Endpoints, serverless bool, pipelineID int) sender.Strategy {
	if endpoints.UseHTTP || serverless {
		encoder := sender.IdentityContentType
		if endpoints.Main.UseCompression {
			encoder = sender.NewGzipContentEncoding(endpoints.Main.CompressionLevel)
		}
		return sender.NewBatchStrategy(inputChan, outputChan, flushChan, sender.ArraySerializer, endpoints.BatchWait, endpoints.BatchMaxSize, endpoints.BatchMaxContentSize, "logs", encoder)
	}
	return sender.NewStreamStrategy(inputChan, outputChan, sender.IdentityContentType)
}
