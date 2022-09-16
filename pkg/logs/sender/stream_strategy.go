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
	inputChan  chan *message.Message
	outputChan chan *message.Payload
	done       chan struct{}
}

// NewStreamStrategy creates a new stream strategy
func NewStreamStrategy(inputChan chan *message.Message, outputChan chan *message.Payload) Strategy {
	return &streamStrategy{
		inputChan:  inputChan,
		outputChan: outputChan,
		done:       make(chan struct{}),
	}
}

// Send sends one message at a time and forwards them to the next stage of the pipeline.
func (s *streamStrategy) Start() {
	go func() {
		for msg := range s.inputChan {
			if msg.Origin != nil {
				msg.Origin.LogSource.LatencyStats.Add(msg.GetLatency())
			}
			s.outputChan <- &message.Payload{Messages: []*message.Message{msg}, Encoded: msg.Content, UnencodedSize: len(msg.Content)}
		}
		s.done <- struct{}{}
	}()
}

// Stop stops the strategy
func (s *streamStrategy) Stop() {
	close(s.inputChan)
	<-s.done
}
