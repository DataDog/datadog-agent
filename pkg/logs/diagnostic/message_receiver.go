// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package diagnostic

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// MessageReceiver interface to handle messages for diagnostics
type MessageReceiver interface {
	HandleMessage(message.Message)
}

// BufferedMessageReceiver handles in coming log messages and makes them available for diagnostics
type BufferedMessageReceiver struct {
	inputChan chan message.Message
	enabled   bool
	m         sync.RWMutex
}

// New creates a new MessageReceiver
func New() *BufferedMessageReceiver {
	return &BufferedMessageReceiver{
		inputChan: make(chan message.Message, config.ChanSize),
	}
}

// Stop closes open channels
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

// SetEnabled start collecting log messages for diagnostics
func (b *BufferedMessageReceiver) SetEnabled(e bool) {
	b.m.Lock()
	defer b.m.Unlock()
	if !e {
		b.clear()
	}
	b.enabled = e
}

// IsEnabled returns the enabled state of the message receiver
func (b *BufferedMessageReceiver) IsEnabled() bool {
	b.m.RLock()
	defer b.m.RUnlock()
	return b.enabled
}

// HandleMessage buffers a message for diagnostic processing
func (b *BufferedMessageReceiver) HandleMessage(m message.Message) {
	if !b.IsEnabled() {
		return
	}
	b.inputChan <- m
}

// Next pops the next buffered event off the input channel formatted as a string
func (b *BufferedMessageReceiver) Next() (line string, ok bool) {
	select {
	case msg := <-b.inputChan:
		return formatMessage(&msg), true
	default:
		return "", false
	}
}

func formatMessage(m *message.Message) string {
	return fmt.Sprintf("%s | %s | %s",
		m.GetStatus(),
		m.Origin.Source(),
		m.Origin.LogSource.Config.Type,
		string(m.Content))
}
