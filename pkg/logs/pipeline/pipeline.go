// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package pipeline

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

// Pipeline processes and sends messages to the backend
type Pipeline struct {
	InputChan chan *message.Message
	processor *processor.Processor
	sender    *sender.Sender
}

// NewPipeline returns a new Pipeline
func NewPipeline(outputChan chan *message.Message, endpoints *config.Endpoints) *Pipeline {
	// initialize the main destination
	main := sender.NewDestination(endpoints.Main)

	// initialize the additional destinations
	var additionals []*sender.Destination
	for _, endpoint := range endpoints.Additionals {
		additionals = append(additionals, sender.NewDestination(endpoint))
	}

	// initialize the sender
	destinations := sender.NewDestinations(main, additionals)
	senderChan := make(chan *message.Message, config.ChanSize)
	sender := sender.NewSender(senderChan, outputChan, destinations)

	// initialize the input chan
	inputChan := make(chan *message.Message, config.ChanSize)

	// initialize the processor
	encoder := processor.NewEncoder(endpoints.Main.UseProto)
	processor := processor.New(inputChan, senderChan, encoder)

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
