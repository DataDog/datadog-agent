// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnostic

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// MessageReceiver interface to handle messages for diagnostics
type MessageReceiver interface {
	HandleMessage(message.Message, []byte)
}

type messagePair struct {
	msg         *message.Message
	redactedMsg []byte
}

// BufferedMessageReceiver handles in coming log messages and makes them available for diagnostics
type BufferedMessageReceiver struct {
	inputChan chan messagePair
	enabled   bool
	m         sync.RWMutex
}

// Filters for processing log messages
type Filters struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Source string `json:"source"`
}

// NewBufferedMessageReceiver creates a new MessageReceiver
func NewBufferedMessageReceiver() *BufferedMessageReceiver {
	return &BufferedMessageReceiver{
		inputChan: make(chan messagePair, config.ChanSize),
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
func (b *BufferedMessageReceiver) HandleMessage(m message.Message, redactedMsg []byte) {
	if !b.IsEnabled() {
		return
	}
	b.inputChan <- messagePair{&m, redactedMsg}
}

// Next pops the next buffered event off the input channel formatted as a string
func (b *BufferedMessageReceiver) Next(filters *Filters) (line string, ok bool) {
	// Read messages until one is handled or none are left
	for {
		select {
		case msgPair := <-b.inputChan:
			if shouldHandleMessage(msgPair.msg, filters) {
				return formatMessage(msgPair.msg, msgPair.redactedMsg), true
			}
			continue
		default:
			return "", false
		}
	}
}

func shouldHandleMessage(m *message.Message, filters *Filters) bool {
	if filters == nil {
		return true
	}

	shouldHandle := true

	if filters.Name != "" {
		shouldHandle = shouldHandle && m.Origin.LogSource.Name == filters.Name
	}

	if filters.Type != "" {
		shouldHandle = shouldHandle && m.Origin.LogSource.Config.Type == filters.Type
	}

	if filters.Source != "" {
		shouldHandle = shouldHandle && filters.Source == m.Origin.Source()
	}

	return shouldHandle
}

func formatMessage(m *message.Message, redactedMsg []byte) string {
	hostname, err := util.GetHostname()
	if err != nil {
		hostname = "unknown"
	}

	ts := time.Now().UTC()
	if !m.Timestamp.IsZero() {
		ts = m.Timestamp
	}

	return fmt.Sprintf("Integration Name: %s | Type: %s | Status: %s | Timestamp: %s | Hostname: %s | Service: %s | Source: %s | Tags: %s | Message: %s\n",
		m.Origin.LogSource.Name,
		m.Origin.LogSource.Config.Type,
		m.GetStatus(),
		ts,
		hostname,
		m.Origin.Service(),
		m.Origin.Source(),
		m.Origin.TagsToString(),
		string(redactedMsg))
}
