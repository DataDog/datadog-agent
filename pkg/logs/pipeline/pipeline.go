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
	InputChan chan message.Message
	processor *processor.Processor
	sender    *sender.Sender
}

// NewPipeline returns a new Pipeline
func NewPipeline(connManager *sender.ConnectionManager, outputChan chan message.Message) *Pipeline {

	var encoder processor.Encoder
	var delimiter sender.Delimiter
	if config.LogsAgent.GetBool("dev_mode_use_proto") {
		encoder = &processor.Proto
		delimiter = &sender.LengthPrefix
	} else {
		encoder = &processor.Raw
		delimiter = &sender.LineBreak
	}

	// initialize the sender
	senderChan := make(chan message.Message, config.ChanSize)
	sender := sender.New(senderChan, outputChan, connManager, delimiter)

	// initialize the input chan
	inputChan := make(chan message.Message, config.ChanSize)

	// initialize the processor
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
