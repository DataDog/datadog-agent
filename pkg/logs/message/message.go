// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

// Message represents a log line sent to datadog, with its metadata
type Message interface {
	Content() []byte
	SetContent([]byte)
	GetOrigin() *Origin
	GetSeverity() []byte
}

type message struct {
	content  []byte
	origin   *Origin
	severity []byte
}

// New returns a new Message
func New(content []byte, origin *Origin, severity []byte) Message {
	return &message{
		content:  content,
		origin:   origin,
		severity: severity,
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

// GetSeverity returns the severity of the message when set
func (m *message) GetSeverity() []byte {
	return m.severity
}
