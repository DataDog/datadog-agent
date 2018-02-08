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
	SetOrigin(*Origin)
	GetTimestamp() string // No need for SetTimestamp as we use Origin under the hood
	GetSeverity() []byte
	SetSeverity([]byte)
}

type message struct {
	content  []byte
	origin   *Origin
	severity []byte
}

func newMessage(content []byte) *message {
	return &message{
		content: content,
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

// SetOrigin sets the integration from which the message comes
func (m *message) SetOrigin(Origin *Origin) {
	m.origin = Origin
}

// GetTimestamp returns the timestamp of the message, or "" if no timestamp is relevant
func (m *message) GetTimestamp() string {
	if m.origin != nil {
		return m.origin.Timestamp
	}
	return ""
}

// GetSeverity returns the severity of the message when set
func (m *message) GetSeverity() []byte {
	return m.severity
}

// SetSeverity sets the severity of the message
func (m *message) SetSeverity(severity []byte) {
	m.severity = severity
}

// FileMessage is a message coming from a File
type FileMessage struct {
	*message
}

// NewFileMessage returns a new FileMessage
func NewFileMessage(content []byte) *FileMessage {
	return &FileMessage{
		message: newMessage(content),
	}
}

// NetworkMessage is a message coming from a network Source
type NetworkMessage struct {
	*message
}

// NewNetworkMessage returns a new NetworkMessage
func NewNetworkMessage(content []byte) *NetworkMessage {
	return &NetworkMessage{
		message: newMessage(content),
	}
}

// ContainerMessage is a message coming from a container Source
type ContainerMessage struct {
	*message
}

// NewContainerMessage returns a new ContainerMessage
func NewContainerMessage(content []byte) *ContainerMessage {
	return &ContainerMessage{
		message: newMessage(content),
	}
}
