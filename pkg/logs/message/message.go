// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package message

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Message represents a log line sent to datadog, with its metadata
type Message struct {
	Content []byte
	Origin  *Origin
	status  string
	// Optional. Must be UTC. If not provided, time.Now().UTC() will be used
	Timestamp time.Time
}

// NewMessageWithSource constructs message with content, status and log source.
func NewMessageWithSource(content []byte, status string, source *config.LogSource) *Message {
	return NewMessage(content, NewOrigin(source), status)
}

// NewMessage constructs message with full information.
func NewMessage(content []byte, origin *Origin, status string) *Message {
	return &Message{
		Content: content,
		Origin:  origin,
		status:  status,
	}
}

// NewMessageWithTime constructs a message with the given information, using the given time for the message timestamp.
func NewMessageWithTime(content []byte, origin *Origin, status string, utcTime time.Time) *Message {
	return &Message{
		Content:   content,
		Origin:    origin,
		status:    status,
		Timestamp: utcTime,
	}
}

// GetStatus gets the status of the message.
// if status is not set, StatusInfo will be returned.
func (m *Message) GetStatus() string {
	if m.status == "" {
		m.status = StatusInfo
	}
	return m.status
}
