// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Sampler is the final stage of the Pipeline. It receives completed log messages
// and emits them to the downstream output, potentially applying rate-limiting or
// sampling logic.
type Sampler interface {
	// Process handles a completed log message.
	Process(msg *message.Message)

	// Flush flushes any buffered state and pending messages.
	Flush()

	// FlushChan returns a channel that delivers a tick when Flush should be called.
	// Returns nil if the sampler has no internal buffering.
	FlushChan() <-chan time.Time
}

// NoopSampler passes all messages directly to the output channel without sampling.
// It is the default implementation used until adaptive sampling logic is complete.
type NoopSampler struct {
	outputChan chan *message.Message
}

// NewNoopSampler returns a new NoopSampler that forwards all messages to outputChan.
func NewNoopSampler(outputChan chan *message.Message) *NoopSampler {
	return &NoopSampler{outputChan: outputChan}
}

// Process passes the message directly to the output channel.
func (s *NoopSampler) Process(msg *message.Message) {
	s.outputChan <- msg
}

// Flush is a no-op since NoopSampler has no buffered state.
func (s *NoopSampler) Flush() {}

// FlushChan returns nil since NoopSampler has no internal buffering.
func (s *NoopSampler) FlushChan() <-chan time.Time {
	return nil
}
