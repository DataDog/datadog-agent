// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package message

import "github.com/DataDog/datadog-agent/pkg/logs/config"

// Message represents a log line sent to datadog, with its metadata
type Message struct {
	Content    []byte
	Origin     *Origin
	RawDataLen int
	Timestamp  string

	status string
}

// NewPartialMessage3 constructs message with partial information.
func NewPartialMessage3(content []byte, origin *Origin, status string) *Message {
	return New(content, origin, status, "", 0)
}

// NewPartialMessage2 constructs message with partial information.
func NewPartialMessage2(content []byte, source *config.LogSource, status string) *Message {
	return NewPartialMessage3(content, NewOrigin(source), status)
}

// NewPartialMessage constructs message with partial information.
func NewPartialMessage(content []byte, status string, timestamp string) *Message {
	return New(content, nil, status, timestamp, 0)
}

// New constructs message with full information.
func New(content []byte, origin *Origin, status string, timestamp string, rawDataLen int) *Message {
	return &Message{
		Content:    content,
		Origin:     origin,
		Timestamp:  timestamp,
		RawDataLen: rawDataLen,

		status: status,
	}
}

// GetStatus gets the status of the message if status is not set, StatusInfo will be returned.
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
