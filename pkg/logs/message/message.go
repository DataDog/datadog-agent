// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

import "github.com/DataDog/datadog-agent/pkg/logs/parser"

// Message represents a log line sent to datadog, with its metadata
type Message interface {
	Content() []byte
	SetContent([]byte)
	GetOrigin() *Origin
	GetStatus() string
}

type message struct {
	content []byte
	origin  *Origin
	status  string
}

// New returns a new Message
func New(content []byte, origin *Origin, status string) Message {
	if status == "" {
		status = parser.StatusInfo
	}
	return &message{
		content: content,
		origin:  origin,
		status:  status,
	}
}

// Content returns the content the message, the actual log line
func (m *message) Content() []byte {
	return m.content
}

// SetContent updates the content the message
func (m *message) SetContent(content []byte) {
	m.content = content
}

// GetOrigin returns the Origin from which the message comes
func (m *message) GetOrigin() *Origin {
	return m.origin
}

// GetStatus returns the status of the message
func (m *message) GetStatus() string {
	return m.status
}
