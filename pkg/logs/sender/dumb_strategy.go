// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sender provides log message sending functionality
package sender

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dumbStrategy is a minimal batching strategy that forwards one message per payload.
type dumbStrategy struct {
	inputChan    chan *message.Message
	outputChan   chan *message.Payload
	flushChan    chan struct{}
	serializer   Serializer
	compression  compression.Compressor
	pipelineName string

	maxContentSize int

	stopChan chan struct{}
	buffer   []*message.Message
}

// type StatefulBatch struct {
//     StateChanges []message.StateChange
//     Logs         []message.Message
//     BatchID      uint64
// }

// NewDumbStrategy returns a strategy that sends one message per payload using the
// provided serializer and compressor. Messages larger than maxContentSize are
// dropped to mimic batch strategy behaviour.
func NewDumbStrategy(
	inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	serializer Serializer,
	maxContentSize int,
	pipelineName string,
	compression compression.Compressor,
) Strategy {
	return &dumbStrategy{
		inputChan:      inputChan,
		outputChan:     outputChan,
		flushChan:      flushChan,
		serializer:     serializer,
		compression:    compression,
		pipelineName:   pipelineName,
		maxContentSize: maxContentSize,
		stopChan:       make(chan struct{}),
		buffer:         make([]*message.Message, 0, 1),
	}
}

// Start begins processing messages from the input channel.
func (s *dumbStrategy) Start() {
	go func() {
		defer close(s.stopChan)
		for {
			select {
			case msg, ok := <-s.inputChan:
				if !ok {
					s.flushBuffer()
					return
				}
				s.bufferMessage(msg)
				s.flushBuffer()
			case <-s.flushChan:
				s.flushBuffer()
			}
		}
	}()
}

// Stop stops the strategy and waits for the processing goroutine to exit.
func (s *dumbStrategy) Stop() {
	close(s.inputChan)
	<-s.stopChan
}

func (s *dumbStrategy) bufferMessage(m *message.Message) {
	if m == nil {
		return
	}

	if s.maxContentSize > 0 && len(m.GetContent()) > s.maxContentSize {
		log.Warnf("Dropped message in pipeline=%s reason=too-large ContentLength=%d ContentSizeLimit=%d", s.pipelineName, len(m.GetContent()), s.maxContentSize)
		tlmDroppedTooLarge.Inc(s.pipelineName)
		return
	}

	s.buffer = append(s.buffer, m)
}

func (s *dumbStrategy) flushBuffer() {

	// TODO: implement flush logic and create a buffer to handle stateful pattern and wildcard patterns.
	// if len(s.buffer) == 0 {
	// 	return
	// }
	// for _, msg := range s.buffer {
	// 	s.processMessage(msg)
	// }
	// for i := range s.buffer {
	// 	s.buffer[i] = nil
	// }
	// s.buffer = s.buffer[:0]

	if len(s.buffer) > 0 {
		s.processMessage(s.buffer[0])
		s.buffer = s.buffer[:0]
	}
}

func (s *dumbStrategy) processMessage(m *message.Message) {
	if m == nil {
		return
	}

	payload, err := s.buildPayload(m)
	if err != nil {
		log.Warn("Encoding failed - dropping payload", err)
		return
	}

	s.outputChan <- payload
}

func (s *dumbStrategy) buildPayload(m *message.Message) (*message.Payload, error) {
	s.serializer.Reset()

	var encodedPayload bytes.Buffer
	compressor := s.compression.NewStreamCompressor(&encodedPayload)
	writer := newWriterWithCounter(compressor)

	if err := s.serializer.Serialize(m, writer); err != nil {
		_ = compressor.Close()
		return nil, err
	}

	if err := s.serializer.Finish(writer); err != nil {
		_ = compressor.Close()
		return nil, err
	}

	if err := compressor.Close(); err != nil {
		return nil, err
	}

	unencodedSize := writer.getWrittenBytes()
	metaCopy := m.MessageMetadata
	payload := message.NewPayload([]*message.MessageMetadata{&metaCopy}, encodedPayload.Bytes(), s.compression.ContentEncoding(), unencodedSize)
	return payload, nil
}
