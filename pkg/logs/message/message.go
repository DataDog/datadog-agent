// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package message

import "github.com/DataDog/datadog-agent/pkg/logs/config"

// Message represents a log line sent to datadog, with its metadata
type Message struct {
	Content []byte
	Origin  *Origin
	status  string
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

// GetStatus gets the status of the message.
// if status is not set, StatusInfo will be returned.
func (m *Message) GetStatus() string {
	if m.status == "" {
		m.status = StatusInfo
	}
	return m.status
}
