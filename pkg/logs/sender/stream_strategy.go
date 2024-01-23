// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// streamStrategy is a Strategy that creates one Payload for each Message, containing
// that Message's Content. This is used for TCP destinations, which stream the output
// without batching multiple messages together.
type streamStrategy struct {
	inputChan       chan *message.Message
	outputChan      chan *message.Payload
	contentEncoding ContentEncoding
	done            chan struct{}
}

// NewStreamStrategy creates a new stream strategy
func NewStreamStrategy(inputChan chan *message.Message, outputChan chan *message.Payload, contentEncoding ContentEncoding) Strategy {
	panic("not called")
}

// Send sends one message at a time and forwards them to the next stage of the pipeline.
func (s *streamStrategy) Start() {
	panic("not called")
}

// Stop stops the strategy
func (s *streamStrategy) Stop() {
	panic("not called")
}
