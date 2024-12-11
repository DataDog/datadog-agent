// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"sync"
)

// Messages holds messages and warning that can be displayed in the status
// Warnings are display at the top of the log section in the status and
// messages are displayed in the log source that generated the message
type Messages struct {
	messages map[string]string
	lock     *sync.Mutex
}

// NewMessages initialize Messages with the default values
func NewMessages() *Messages {
	return &Messages{
		messages: make(map[string]string),
		lock:     &sync.Mutex{},
	}
}

// AddMessage create a message
func (m *Messages) AddMessage(key string, message string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.messages[key] = message
}

// GetMessages returns all the messages
func (m *Messages) GetMessages() []string {
	m.lock.Lock()
	defer m.lock.Unlock()
	messages := make([]string, 0)
	for _, message := range m.messages {
		messages = append(messages, message)
	}
	return messages
}

// RemoveMessage removes a message
func (m *Messages) RemoveMessage(key string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.messages, key)
}
