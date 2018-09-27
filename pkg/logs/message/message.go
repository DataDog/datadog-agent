// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

import "github.com/DataDog/datadog-agent/pkg/logs/severity"

// Message represents a log line sent to datadog, with its metadata
type Message struct {
	Content    []byte
	Origin     *Origin
	Status     string
	Timestamp  string
	RawDataLen int
}

// GetOrigin returns the Origin from which the message comes
func (m *Message) GetOrigin() *Origin {
	return m.Origin
}

// SetOrigin sets the Origin from which the message comes
func (m *Message) SetOrigin(origin *Origin) {
	m.Origin = origin
}

// GetStatus returns the status of the message
func (m *Message) GetStatus() string {
	if m.Status == "" {
		m.Status = severity.StatusInfo
	}
	return m.Status
}

// SetStatus sets the status of the message
func (m *Message) SetStatus(status string) {
	if status == "" {
		status = severity.StatusInfo
	}
	m.Status = status
}
