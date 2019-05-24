// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package pipeline

import (
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

// Pipeline processes and sends messages to the backend
type Pipeline struct {
	InputChan chan *message.Message
	processor *processor.Processor
	sender    sender.Sender
}

// NewPipeline returns a new Pipeline
func NewPipeline(outputChan chan *message.Message, processingRules []*config.ProcessingRule, endpoints *config.Endpoints, destinationsContext *tcp.DestinationsContext) *Pipeline {
	// initialize the sender
	destinations := client.NewDestinations(endpoints, destinationsContext)
	senderChan := make(chan *message.Message, config.ChanSize)
	var newsender sender.Sender
	if coreConfig.Datadog.GetBool("logs_config.use_http") {
		newsender = sender.NewBatchSender(senderChan, outputChan, destinations)
	} else {
		newsender = sender.NewSingleSender(senderChan, outputChan, destinations)
	}

	// initialize the input chan
	inputChan := make(chan *message.Message, config.ChanSize)

	// initialize the processor
	var encoder processor.Encoder
	if coreConfig.Datadog.GetBool("logs_config.use_http") {
		encoder = processor.NewHTTPEncoder()
	} else {
		encoder = processor.NewTCPEncoder(endpoints.Main.UseProto)
	}
	processor := processor.New(inputChan, senderChan, processingRules, encoder)

	return &Pipeline{
		InputChan: inputChan,
		processor: processor,
		sender:    newsender,
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
