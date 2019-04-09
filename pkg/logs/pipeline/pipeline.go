// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package pipeline

import (
	"github.com/StackVista/stackstate-agent/pkg/logs/client"
	"github.com/StackVista/stackstate-agent/pkg/logs/config"
	"github.com/StackVista/stackstate-agent/pkg/logs/message"
	"github.com/StackVista/stackstate-agent/pkg/logs/processor"
	"github.com/StackVista/stackstate-agent/pkg/logs/sender"
)

// Pipeline processes and sends messages to the backend
type Pipeline struct {
	InputChan chan *message.Message
	processor *processor.Processor
	sender    *sender.Sender
}

// NewPipeline returns a new Pipeline
func NewPipeline(outputChan chan *message.Message, processingRules []*config.ProcessingRule, endpoints *client.Endpoints, destinationsContext *client.DestinationsContext) *Pipeline {
	// initialize the main destination
	main := client.NewDestination(endpoints.Main, destinationsContext)

	// initialize the additional destinations
	var additionals []*client.Destination
	for _, endpoint := range endpoints.Additionals {
		additionals = append(additionals, client.NewDestination(endpoint, destinationsContext))
	}

	// initialize the sender
	destinations := client.NewDestinations(main, additionals)
	senderChan := make(chan *message.Message, config.ChanSize)
	sender := sender.NewSender(senderChan, outputChan, destinations)

	// initialize the input chan
	inputChan := make(chan *message.Message, config.ChanSize)

	// initialize the processor
	encoder := processor.NewEncoder(endpoints.Main.UseProto)
	processor := processor.New(inputChan, senderChan, processingRules, encoder)

	return &Pipeline{
		InputChan: inputChan,
		processor: processor,
		sender:    sender,
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
