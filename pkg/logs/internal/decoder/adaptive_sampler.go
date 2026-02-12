// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package decoder

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// AdaptiveSampler applies adaptive sampling to log messages based on their token patterns.
// It analyzes message tokens (from ParsingExtra.Tokens) to identify rare vs common patterns
// and applies rate limiting accordingly.
type AdaptiveSampler interface {
	// process handles a log message and determines whether to emit it based on sampling rules
	process(*message.Message)

	// flushChan returns a channel which will deliver a message when `flush` should be called.
	flushChan() <-chan time.Time

	// flush flushes any buffered state and pending messages.
	flush()
}

// NoopSampler provides a passthrough sampler that emits all messages without sampling.
// This is used as the default implementation until adaptive sampling logic is complete.
type NoopSampler struct {
	outputChan chan *message.Message
}

// NewNoopSampler returns a new NoopSampler that passes all messages to the output channel.
func NewNoopSampler(outputChan chan *message.Message) *NoopSampler {
	return &NoopSampler{outputChan: outputChan}
}

// process passes the message directly to the output channel without sampling
func (s *NoopSampler) process(msg *message.Message) {
	s.outputChan <- msg
}

// flushChan returns nil since NoopSampler has no buffering
func (s *NoopSampler) flushChan() <-chan time.Time {
	return nil
}

// flush is a no-op since NoopSampler has no buffered state
func (s *NoopSampler) flush() {
	// No buffered state to flush
}
