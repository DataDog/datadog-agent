// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package pipeline

import (
	"context"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
)

// Pipeline processes and sends messages to the backend
type Pipeline struct {
	InputChan       chan *message.Message
	flushChan       chan struct{}
	processor       *processor.Processor
	strategy        sender.Strategy
	sender          sender.PipelineComponent
	serverless      bool
	flushWg         *sync.WaitGroup
	pipelineMonitor metrics.PipelineMonitor
}

// NewPipeline returns a new Pipeline
// When sharedSender is not provided, this func creates a sender for the created pipeline.
func NewPipeline(outputChan chan *message.Payload,
	processingRules []*config.ProcessingRule,
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	auditor auditor.Auditor,
	sharedSender sender.PipelineComponent,
	diagnosticMessageReceiver diagnostic.MessageReceiver,
	serverless bool,
	pipelineID int,
	status statusinterface.Status,
	hostname hostnameinterface.Component,
	cfg pkgconfigmodel.Reader) *Pipeline {

	var flushWg *sync.WaitGroup
	var senderDoneChan chan *sync.WaitGroup
	if serverless {
		senderDoneChan = make(chan *sync.WaitGroup)
		flushWg = &sync.WaitGroup{}
	}

	strategyInput := make(chan *message.Message, pkgconfigsetup.Datadog().GetInt("logs_config.message_channel_size"))
	flushChan := make(chan struct{})

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

	// if not provided, create a sender for this pipeline
	var senderImpl sender.PipelineComponent
	if sharedSender == nil {
		pipelineMonitor := metrics.NewTelemetryPipelineMonitor(strconv.Itoa(pipelineID))
		mainDestinations := GetDestinations(endpoints, destinationsContext, pipelineMonitor, serverless, senderDoneChan, status, cfg)
		senderInput := make(chan *message.Payload, 1) // only buffer 1 message since payloads can be large
		senderImpl = sender.NewSender(cfg, senderInput, auditor, mainDestinations,
			pkgconfigsetup.Datadog().GetInt("logs_config.payload_channel_size"), senderDoneChan, flushWg, pipelineMonitor)
	} else {
		senderImpl = sharedSender
	}

	strategy := getStrategy(strategyInput, senderImpl.In(), flushChan, endpoints, serverless, flushWg, senderImpl.PipelineMonitor())

	inputChan := make(chan *message.Message, pkgconfigsetup.Datadog().GetInt("logs_config.message_channel_size"))
	processor := processor.New(cfg, inputChan, strategyInput, processingRules,
		encoder, diagnosticMessageReceiver, hostname, senderImpl.PipelineMonitor())

	return &Pipeline{
		InputChan:       inputChan,
		flushChan:       flushChan,
		processor:       processor,
		strategy:        strategy,
		sender:          senderImpl,
		serverless:      serverless,
		flushWg:         flushWg,
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

	if p.serverless {
		// Wait for the logs sender to finish sending payloads to all destinations before allowing the flush to finish
		p.flushWg.Wait()
	}
}

//nolint:revive // TODO(AML) Fix revive linter
func getStrategy(inputChan chan *message.Message, outputChan chan *message.Payload, flushChan chan struct{}, endpoints *config.Endpoints, serverless bool, flushWg *sync.WaitGroup, pipelineMonitor metrics.PipelineMonitor) sender.Strategy {
	if endpoints.UseHTTP || serverless {
		encoder := sender.IdentityContentType
		if endpoints.Main.UseCompression {
			encoder = sender.NewGzipContentEncoding(endpoints.Main.CompressionLevel)
		}
		return sender.NewBatchStrategy(inputChan, outputChan, flushChan, serverless, flushWg, sender.ArraySerializer, endpoints.BatchWait, endpoints.BatchMaxSize, endpoints.BatchMaxContentSize, "logs", encoder, pipelineMonitor)
	}
	return sender.NewStreamStrategy(inputChan, outputChan, sender.IdentityContentType)
}

// GetDestinations returns configured destinations instances for the given endpoints.
func GetDestinations(endpoints *config.Endpoints, destinationsContext *client.DestinationsContext, pipelineMonitor metrics.PipelineMonitor, serverless bool, senderDoneChan chan *sync.WaitGroup, status statusinterface.Status, cfg pkgconfigmodel.Reader) *client.Destinations {
	reliable := []client.Destination{}
	additionals := []client.Destination{}

	if endpoints.UseHTTP {
		for i, endpoint := range endpoints.GetReliableEndpoints() {
			destMeta := client.NewDestinationMetadata("logs", pipelineMonitor.ID(), "reliable", strconv.Itoa(i))
			if serverless {
				reliable = append(reliable, http.NewSyncDestination(endpoint, http.JSONContentType, destinationsContext, senderDoneChan, destMeta, cfg))
			} else {
				reliable = append(reliable, http.NewDestination(endpoint, http.JSONContentType, destinationsContext, endpoints.BatchMaxConcurrentSend, true, destMeta, cfg, pipelineMonitor))
			}
		}
		for i, endpoint := range endpoints.GetUnReliableEndpoints() {
			destMeta := client.NewDestinationMetadata("logs", pipelineMonitor.ID(), "unreliable", strconv.Itoa(i))
			if serverless {
				additionals = append(additionals, http.NewSyncDestination(endpoint, http.JSONContentType, destinationsContext, senderDoneChan, destMeta, cfg))
			} else {
				additionals = append(additionals, http.NewDestination(endpoint, http.JSONContentType, destinationsContext, endpoints.BatchMaxConcurrentSend, false, destMeta, cfg, pipelineMonitor))
			}
		}
		return client.NewDestinations(reliable, additionals)
	}
	for _, endpoint := range endpoints.GetReliableEndpoints() {
		reliable = append(reliable, tcp.NewDestination(endpoint, endpoints.UseProto, destinationsContext, !serverless, status))
	}
	for _, endpoint := range endpoints.GetUnReliableEndpoints() {
		additionals = append(additionals, tcp.NewDestination(endpoint, endpoints.UseProto, destinationsContext, false, status))
	}

	return client.NewDestinations(reliable, additionals)
}
