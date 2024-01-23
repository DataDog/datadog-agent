// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnostic

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// MessageReceiver interface to handle messages for diagnostics
type MessageReceiver interface {
	HandleMessage(*message.Message, []byte, string)
}

type messagePair struct {
	msg       *message.Message
	rendered  []byte
	eventType string
}

// BufferedMessageReceiver handles in coming log messages and makes them available for diagnostics
type BufferedMessageReceiver struct {
	inputChan chan messagePair
	enabled   bool
	m         sync.RWMutex
	formatter Formatter
}

// Filters for processing log messages
type Filters struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Source  string `json:"source"`
	Service string `json:"service"`
}

// NewBufferedMessageReceiver creates a new MessageReceiver. It takes an optional Formatter as a parameter, and defaults
// to using logFormatter if not supplied.
func NewBufferedMessageReceiver(f Formatter) *BufferedMessageReceiver {
	if f == nil {
		f = &logFormatter{}
	}
	return &BufferedMessageReceiver{
		inputChan: make(chan messagePair, config.ChanSize),
		formatter: f,
	}
}

// Start opens new input channel
func (b *BufferedMessageReceiver) Start() {
	b.inputChan = make(chan messagePair, config.ChanSize)
}

// Stop closes the input channel
func (b *BufferedMessageReceiver) Stop() {
	panic("not called")
}

// Clear empties buffered messages
func (b *BufferedMessageReceiver) clear() {
	panic("not called")
}

// SetEnabled start collecting log messages for diagnostics. Returns true if state was successfully changed
func (b *BufferedMessageReceiver) SetEnabled(e bool) bool {
	panic("not called")
}

// IsEnabled returns the enabled state of the message receiver
func (b *BufferedMessageReceiver) IsEnabled() bool {
	panic("not called")
}

// HandleMessage buffers a message for diagnostic processing
func (b *BufferedMessageReceiver) HandleMessage(m *message.Message, rendered []byte, eventType string) {
	panic("not called")
}

// Filter writes the buffered events from the input channel formatted as a string to the output channel
func (b *BufferedMessageReceiver) Filter(filters *Filters, done <-chan struct{}) <-chan string {
	panic("not called")
}

func shouldHandleMessage(m *messagePair, filters *Filters) bool {
	panic("not called")
}
