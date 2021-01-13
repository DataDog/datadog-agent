// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package diagnostic

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// MessageReceiver handles in coming log messages and makes them available for diagnostics
type MessageReceiver struct {
	inputChan chan message.Message
	done      chan struct{}
}

// New creates a new MessageReceiver
func New() *MessageReceiver {
	return &MessageReceiver{
		inputChan: make(chan message.Message, 100),
	}
}

// Stop closes open channels
func (d *MessageReceiver) Stop() {
	close(d.inputChan)
}

// Clear empties buffered messages
func (d *MessageReceiver) Clear() {
	l := len(d.inputChan)
	for i := 0; i < l; i++ {
		<-d.inputChan
	}
}

// Next pops the next buffered event off the input channel formatted as a string
func (d *MessageReceiver) Next() (line string, ok bool) {
	select {
	case msg := <-d.inputChan:
		return formatMessage(&msg), true
	default:
		return "", false
	}
}

// Channel gets the input channel
func (d *MessageReceiver) Channel() chan message.Message {
	return d.inputChan
}

func formatMessage(m *message.Message) string {
	return fmt.Sprintf("%s | %s | %s", m.Origin.Source(), m.Origin.LogSource.Config.Type, string(m.Content))
}
