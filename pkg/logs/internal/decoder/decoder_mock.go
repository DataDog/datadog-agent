// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// MockDecoder is a mock decoder that can be used to test the decoder
type MockDecoder struct {
	inputChan  chan *message.Message
	outputChan chan *message.Message
}

// InputChan returns the input channel
func (d *MockDecoder) InputChan() chan *message.Message {
	return d.inputChan
}

// OutputChan returns the output channel
func (d *MockDecoder) OutputChan() chan *message.Message {
	return d.outputChan
}

// Start starts the mock decoder
func (d *MockDecoder) Start() {
	go d.run()
}

// Stop stops the mock decoder
func (d *MockDecoder) Stop() {
	close(d.inputChan)
}

func (d *MockDecoder) run() {
	for msg := range d.inputChan {
		d.outputChan <- msg
	}
}

// GetDetectedPattern returns the detected pattern (if any)
func (d *MockDecoder) GetDetectedPattern() *regexp.Regexp {
	return nil
}

// GetLineCount returns the number of decoded lines
func (d *MockDecoder) GetLineCount() int64 {
	return 0
}

// MockDecoderOptions are the options for creating a mock decoder
type MockDecoderOptions struct {
	InputChanSize  int
	OutputChanSize int
}

// NewMockDecoderWithOptions creates a new mock decoder with the given options
func NewMockDecoderWithOptions(options *MockDecoderOptions) *MockDecoder {
	if options == nil {
		return NewMockDecoder()
	}
	return &MockDecoder{
		inputChan:  make(chan *message.Message, options.InputChanSize),
		outputChan: make(chan *message.Message, options.OutputChanSize),
	}
}

// NewMockDecoder creates a new mock decoder
func NewMockDecoder() *MockDecoder {
	return &MockDecoder{
		inputChan:  make(chan *message.Message, 10),
		outputChan: make(chan *message.Message, 10),
	}
}
