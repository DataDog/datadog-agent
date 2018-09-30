// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

// Message represents a log line sent to datadog, with its metadata
type Message struct {
	Content    []byte
	Origin     *Origin
	status     string
	Timestamp  string
	RawDataLen int
}

func NewMessage() *Message {
	return &Message{}
}

// GetStatus returns the status of the message
func (m *Message) GetStatus() string {
	if m.status == "" {
		m.status = StatusInfo
	}
	return m.status
}

// SetStatus sets the status of the message
func (m *Message) SetStatus(status string) {
	m.status = status
}
