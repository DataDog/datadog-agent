// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

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
		m.Status = StatusInfo
	}
	return m.Status
}

// SetStatus sets the status of the message
func (m *Message) SetStatus(status string) {
	if status == "" {
		status = StatusInfo
	}
	m.Status = status
}
