// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package decoder

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// NoopLineHandler provides a passthrough functionality for flows that don't need a functional line handler
type NoopLineHandler struct {
	outputChan chan *message.Message
}

// NewNoopLineHandler returns a new NoopLineHandler
func NewNoopLineHandler(outputChan chan *message.Message) *NoopLineHandler {
	return &NoopLineHandler{outputChan: outputChan}
}

// process handles a new line (message)
func (noop *NoopLineHandler) process(msg *message.Message) {
	noop.outputChan <- msg
}

// flushChan returns a channel which will deliver a message when `flush` should be called.
func (noop *NoopLineHandler) flushChan() <-chan time.Time {
	return nil
}

// flush flushes partially-processed data.  It should be called either when flushChan has
// a message, or when the decoder is stopped.
func (noop *NoopLineHandler) flush() {

}
