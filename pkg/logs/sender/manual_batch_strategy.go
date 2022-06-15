// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type manualBatchStrategy struct {
	strategy  *batchStrategy
	flushChan chan struct{}
}

// NewManualBatchStrategy returns a new batch concurrent strategy with the specified batch & content size limits
func NewManualBatchStrategy(inputChan chan *message.Message,
	outputChan chan *message.Payload,
	serializer Serializer,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	contentEncoding ContentEncoding) Strategy {

	return &manualBatchStrategy{
		strategy: &batchStrategy{
			inputChan:       inputChan,
			outputChan:      outputChan,
			buffer:          NewMessageBuffer(maxBatchSize, maxContentSize),
			serializer:      serializer,
			contentEncoding: contentEncoding,
			stopChan:        make(chan struct{}),
			pipelineName:    pipelineName,
		},
		flushChan: make(chan struct{}),
	}
}

// Start reads the incoming messages and accumulates them to a buffer
func (s *manualBatchStrategy) Start() {
	go func() {
		defer func() {
			s.strategy.flushBuffer(s.strategy.outputChan)
			close(s.strategy.stopChan)
		}()
		for {
			select {
			case m, isOpen := <-s.strategy.inputChan:

				if !isOpen {
					// inputChan has been closed, no more payloads are expected
					return
				}
				s.strategy.processMessage(m, s.strategy.outputChan)
			case <-s.flushChan:
				s.strategy.flushBuffer(s.strategy.outputChan)
			}
		}
	}()
}

// Stop flushes the buffer and stops the strategy
func (s *manualBatchStrategy) Stop() {
	s.strategy.Stop()
}

// Flush flushes the buffer
func (s *manualBatchStrategy) Flush() {
	s.flushChan <- struct{}{}
}
