// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/security/log"
)

// Pipeline processes and sends messages to the backend
type Pipeline struct {
	InputChan chan *message.Message
	processor *processor.Processor
	sender    sender.Sender
}

// NewPipeline returns a new Pipeline
func NewPipeline(outputChan chan *message.Message, processingRules []*config.ProcessingRule, endpoints *config.Endpoints, destinationsContext *client.DestinationsContext, diagnosticMessageReceiver diagnostic.MessageReceiver, serverless bool, pipelineID int) *Pipeline {
	mainDestinations := getMainDestinations(endpoints, destinationsContext)
	reliableAdditionalDestinations := getReliableAdditionalDestinations(endpoints, destinationsContext)

	senderChan := make(chan *message.Message, config.ChanSize)

	var logSender sender.Sender

	// If there is a reliable additional endpoint - we are dual-shipping so we need to spawn an additional sender.
	if reliableAdditionalDestinations != nil {
		mainSender := sender.NewSingleSender(make(chan *message.Message, config.ChanSize), outputChan, mainDestinations, getStrategy(endpoints, serverless, pipelineID))
		additionalSender := sender.NewSingleSender(make(chan *message.Message, config.ChanSize), outputChan, reliableAdditionalDestinations, getStrategy(endpoints, serverless, pipelineID))

		logSender = sender.NewDualSender(senderChan, mainSender, additionalSender)
	} else {
		logSender = sender.NewSingleSender(senderChan, outputChan, mainDestinations, getStrategy(endpoints, serverless, pipelineID))
	}

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

	inputChan := make(chan *message.Message, config.ChanSize)
	processor := processor.New(inputChan, senderChan, processingRules, encoder, diagnosticMessageReceiver)

	return &Pipeline{
		InputChan: inputChan,
		processor: processor,
		sender:    logSender,
	}
}

// Start launches the pipeline
func (p *Pipeline) Start() {
	p.sender.Start()
	p.processor.Start()
}

// Stop stops the pipeline
func (p *Pipeline) Stop() {
	p.processor.Stop()
	p.sender.Stop()
}

// Flush flushes synchronously the processor and sender managed by this pipeline.
func (p *Pipeline) Flush(ctx context.Context) {
	p.processor.Flush(ctx) // flush messages in the processor into the sender
	p.sender.Flush(ctx)    // flush the sender
}

func getMainDestinations(endpoints *config.Endpoints, destinationsContext *client.DestinationsContext) *client.Destinations {
	if endpoints.UseHTTP {
		main := http.NewDestination(endpoints.Main, http.JSONContentType, destinationsContext, endpoints.BatchMaxConcurrentSend)
		additionals := []client.Destination{}
		for _, endpoint := range endpoints.GetUnReliableAdditionals() {
			additionals = append(additionals, http.NewDestination(endpoint, http.JSONContentType, destinationsContext, endpoints.BatchMaxConcurrentSend))
		}
		return client.NewDestinations(main, additionals)
	}
	main := tcp.NewDestination(endpoints.Main, endpoints.UseProto, destinationsContext)
	additionals := []client.Destination{}
	for _, endpoint := range endpoints.GetUnReliableAdditionals() {
		additionals = append(additionals, tcp.NewDestination(endpoint, endpoints.UseProto, destinationsContext))
	}
	return client.NewDestinations(main, additionals)
}

func getReliableAdditionalDestinations(endpoints *config.Endpoints, destinationsContext *client.DestinationsContext) *client.Destinations {
	reliableEndpoints := endpoints.GetReliableAdditionals()
	var reliableAdditionalEndpoint config.Endpoint

	if len(reliableEndpoints) >= 1 {
		log.Infof("Found an additional reliable endpoint. Only the first additional endpoint marked as reliable will be used at this time.")
		reliableAdditionalEndpoint = reliableEndpoints[0]
	} else {
		return nil
	}

	if endpoints.UseHTTP {
		backup := http.NewDestination(reliableAdditionalEndpoint, http.JSONContentType, destinationsContext, endpoints.BatchMaxConcurrentSend)
		return client.NewDestinations(backup, []client.Destination{})
	}
	backup := tcp.NewDestination(reliableAdditionalEndpoint, endpoints.UseProto, destinationsContext)
	return client.NewDestinations(backup, []client.Destination{})
}

func getStrategy(endpoints *config.Endpoints, serverless bool, pipelineID int) sender.Strategy {
	if endpoints.UseHTTP || serverless {
		return sender.NewBatchStrategy(sender.ArraySerializer, endpoints.BatchWait, endpoints.BatchMaxConcurrentSend, endpoints.BatchMaxSize, endpoints.BatchMaxContentSize, "logs", pipelineID)
	}
	return sender.StreamStrategy
}
