// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package message

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Message represents a log line sent to datadog, with its metadata
type Message interface {
	Content() []byte
	SetContent([]byte)
	GetOrigin() *MessageOrigin
	SetOrigin(*MessageOrigin)
	GetTimestamp() string // No need for SetTimestamp as we use MessageOrigin under the hood
	GetSeverity() []byte
	SetSeverity([]byte)
	GetTagsPayload() []byte
	SetTagsPayload([]byte)
}

// MessageOrigin represents the Origin of a message
type MessageOrigin struct {
	Identifier string
	LogSource  *config.IntegrationConfigLogSource
	Offset     int64
	Timestamp  string
}

type message struct {
	content     []byte
	Origin      *MessageOrigin
	severity    []byte
	tagsPayload []byte
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
func (m *message) GetOrigin() *MessageOrigin {
	return m.Origin
}

// SetOrigin sets the integration from which the message comes
func (m *message) SetOrigin(Origin *MessageOrigin) {
	m.Origin = Origin
}

// GetTimestamp returns the timestamp of the message, or "" if no timestamp is relevant
func (m *message) GetTimestamp() string {
	if m.Origin != nil {
		return m.Origin.Timestamp
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

// GetSeverity returns the tags and sources of the message
// It will default on the LogSource tags payload, but can
// be overriden in the message itself with tagsPayload
func (m *message) GetTagsPayload() []byte {
	if m.tagsPayload != nil {
		return m.tagsPayload
	}
	if m.Origin != nil && m.Origin.LogSource != nil {
		return m.Origin.LogSource.TagsPayload
	}
	return nil
}

// SetSeverity sets the tags and sources of the message
func (m *message) SetTagsPayload(tagsPayload []byte) {
	m.tagsPayload = tagsPayload
}

// NewMessage returns a new message
func NewMessage(content []byte) *message {
	return &message{
		content: content,
	}
}

// NewFileOrigin returns a new MessageOrigin
func NewOrigin() *MessageOrigin {
	return &MessageOrigin{}
}

// StopMessage is used to let a component stop gracefully
type StopMessage struct {
	*message
}

func NewStopMessage() *StopMessage {
	return &StopMessage{
		message: NewMessage(nil),
	}
}

// FileMessage is a message coming from a File
type FileMessage struct {
	*message
}

func NewFileMessage(content []byte) *FileMessage {
	return &FileMessage{
		message: NewMessage(content),
	}
}

// FileMessage is a message coming from a network Source
type NetworkMessage struct {
	*message
}

func NewNetworkMessage(content []byte) *NetworkMessage {
	return &NetworkMessage{
		message: NewMessage(content),
	}
}

// ContainerMessage is a message coming from a container Source
type ContainerMessage struct {
	*message
}

func NewContainerMessage(content []byte) *ContainerMessage {
	return &ContainerMessage{
		message: NewMessage(content),
	}
}
