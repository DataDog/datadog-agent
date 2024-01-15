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
	close(b.inputChan)
}

// Clear empties buffered messages
func (b *BufferedMessageReceiver) clear() {
	l := len(b.inputChan)
	for i := 0; i < l; i++ {
		<-b.inputChan
	}
}

// SetEnabled start collecting log messages for diagnostics. Returns true if state was successfully changed
func (b *BufferedMessageReceiver) SetEnabled(e bool) bool {
	b.m.Lock()
	defer b.m.Unlock()

	if b.enabled == e {
		return false
	}

	b.enabled = e
	if !e {
		b.clear()
	}
	return true
}

// IsEnabled returns the enabled state of the message receiver
func (b *BufferedMessageReceiver) IsEnabled() bool {
	b.m.RLock()
	defer b.m.RUnlock()
	return b.enabled
}

// HandleMessage buffers a message for diagnostic processing
func (b *BufferedMessageReceiver) HandleMessage(m *message.Message, rendered []byte, eventType string) {
	if !b.IsEnabled() {
		return
	}
	b.inputChan <- messagePair{
		msg:       m,
		rendered:  rendered,
		eventType: eventType,
	}
}

// Filter writes the buffered events from the input channel formatted as a string to the output channel
func (b *BufferedMessageReceiver) Filter(filters *Filters, done <-chan struct{}) <-chan string {
	out := make(chan string, config.ChanSize)
	go func() {
		defer close(out)
		for {
			select {
			case msgPair := <-b.inputChan:
				if shouldHandleMessage(&msgPair, filters) {
					out <- b.formatter.Format(msgPair.msg, msgPair.eventType, msgPair.rendered)
				}
			case <-done:
				return
			}
		}
	}()
	return out
}

func shouldHandleMessage(m *messagePair, filters *Filters) bool {
	if filters == nil {
		return true
	}

	shouldHandle := true

	if filters.Name != "" {
		shouldHandle = shouldHandle && m.msg.Origin != nil && m.msg.Origin.LogSource.Name == filters.Name
	}

	if filters.Type != "" {
		shouldHandle = shouldHandle && ((m.msg.Origin != nil && m.msg.Origin.LogSource.Config.Type == filters.Type) || m.eventType == filters.Type)
	}

	if filters.Source != "" {
		shouldHandle = shouldHandle && m.msg.Origin != nil && filters.Source == m.msg.Origin.Source()
	}

	if filters.Service != "" {
		shouldHandle = shouldHandle && m.msg.Origin != nil && filters.Service == m.msg.Origin.Service()
	}

	return shouldHandle
}
