// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package windowsevent

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Message implements StructedMessage interface for Windows Event Log messages.
type Message struct { //nolint:revive
	data *Map
}

// Render renders the structured log information into JSON, for further encoding before
// being sent to the intake.
func (m *Message) Render() ([]byte, error) {
	data, err := m.data.Json()
	if err != nil {
		return nil, err
	}
	log.Trace("Rendered JSON in structured message:", string(data))
	return data, nil
}

// GetContent returns the content part of the structured log.
func (m *Message) GetContent() []byte {
	message := m.data.GetMessage()
	if message == "" {
		log.Error("Message not containing any message")
		return []byte{}
	}
	return []byte(message)
}

// SetContent sets the content part of the structured log.
func (m *Message) SetContent(content []byte) {
	// we want to store it typed as a string for the json
	// marshaling to properly marshal it as a string.
	_ = m.data.SetMessage(string(content))
}

// MapToMessage packages a Map into either an unstructured message.Message or a structured one.
func MapToMessage(m *Map, source *sources.LogSource, processRawMessage bool) (*message.Message, error) {
	// old behaviour using an unstructured message with raw data
	if processRawMessage {
		jsonEvent, err := m.Json()
		if err != nil {
			return nil, err
		}
		return message.NewMessageWithSource(jsonEvent, message.StatusInfo, source, time.Now().UnixNano()), nil
	}

	// new behaviour returning a structured message
	return message.NewStructuredMessage(
		&Message{data: m},
		message.NewOrigin(source),
		message.StatusInfo,
		time.Now().UnixNano(),
	), nil
}
