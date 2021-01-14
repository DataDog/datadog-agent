// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package diagnostic

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

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
		inputChan: make(chan message.Message, 100),
	}
}

// Stop closes open channels
func (d *BufferedMessageReceiver) Stop() {
	close(d.inputChan)
}

// Clear empties buffered messages
func (d *BufferedMessageReceiver) clear() {
	l := len(d.inputChan)
	for i := 0; i < l; i++ {
		<-d.inputChan
	}
}

func (b *BufferedMessageReceiver) SetEnabled(e bool) {
	b.m.Lock()
	defer b.m.Unlock()
	if !e {
		b.clear()
	}
	b.enabled = e
}

func (b *BufferedMessageReceiver) HandleMessage(m message.Message) {
	b.m.RLock()
	if !b.enabled {
		return
	}
	b.m.RUnlock()
	b.inputChan <- m
}

// Next pops the next buffered event off the input channel formatted as a string
func (d *BufferedMessageReceiver) Next() (line string, ok bool) {
	select {
	case msg := <-d.inputChan:
		return formatMessage(&msg), true
	default:
		return "", false
	}
}

func formatMessage(m *message.Message) string {
	return fmt.Sprintf("%s | %s | %s", m.Origin.Source(), m.Origin.LogSource.Config.Type, string(m.Content))
}
