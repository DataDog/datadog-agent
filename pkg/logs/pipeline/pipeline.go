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

	useProto := config.LogsAgent.GetBool("logs_config.dev_mode_use_proto")

	// initialize the sender
	senderChan := make(chan message.Message, config.ChanSize)
	delimiter := sender.NewDelimiter(useProto)
	sender := sender.New(senderChan, outputChan, connManager, delimiter)

	// initialize the input chan
	inputChan := make(chan message.Message, config.ChanSize)

	// initialize the processor
	encoder := processor.NewEncoder(useProto)
	apikey := config.LogsAgent.GetString("api_key")
	logset := config.LogsAgent.GetString("logset") // TODO Logset is deprecated and should be removed eventually.
	prefixer := processor.NewAPIKeyPrefixer(apikey, logset)
	processor := processor.New(inputChan, senderChan, encoder, prefixer)

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
