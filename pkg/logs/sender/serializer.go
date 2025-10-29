// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Serializer transforms a batch of messages into a payload.
// It is the one rendering the messages (i.e. either directly using
// raw []byte data from unstructured messages or turning structured
// messages into []byte data).
type Serializer interface {
	Serialize(message *message.Message, writer io.Writer) error
	Finish(writer io.Writer) error
	Reset()
}

// arraySerializer transforms a message array into a array string payload.
type arraySerializer struct {
	isFirstMessage bool
}

// NewArraySerializer creates a new arraySerializer
func NewArraySerializer() Serializer {
	return &arraySerializer{isFirstMessage: true}
}

// Serialize transforms all messages into a array string
// for example:
// "{"message":"content1"}", "{"message":"content2"}"
// returns, "[{"message":"content1"},{"message":"content2"}]"
func (s *arraySerializer) Serialize(message *message.Message, writer io.Writer) error {
	if s.isFirstMessage {
		if _, err := writer.Write([]byte{'['}); err != nil {
			return err
		}
		s.isFirstMessage = false
	} else {
		if _, err := writer.Write([]byte{','}); err != nil {
			return err
		}
	}
	_, err := writer.Write(message.GetContent())
	return err
}

// Finish writes the closing bracket for JSON array
func (s *arraySerializer) Finish(writer io.Writer) error {
	_, err := writer.Write([]byte{']'})
	s.Reset()
	return err
}

// Reset resets the serializer to its initial state
func (s *arraySerializer) Reset() {
	s.isFirstMessage = true
}

// singleObjectSerializer sends messages as individual objects without array wrapping.
// This is used for APIs that expect single objects rather than arrays.
// It only supports sending one message per batch.
type singleObjectSerializer struct {
	messageCount int
}

// NewSingleObjectSerializer creates a new singleObjectSerializer
func NewSingleObjectSerializer() Serializer {
	return &singleObjectSerializer{messageCount: 0}
}

// Serialize writes a single message without array wrapping.
// If called multiple times, it will log a warning since this serializer
// is designed for single-object APIs.
func (s *singleObjectSerializer) Serialize(message *message.Message, writer io.Writer) error {
	if s.messageCount > 0 {
		// This shouldn't happen if batch_max_size is set to 1
		// But we log it defensively
		log.Warn("singleObjectSerializer received multiple messages in one batch, only the last will be sent")
	}
	s.messageCount++
	_, err := writer.Write(message.GetContent())
	return err
}

// Finish completes the serialization (no-op for single objects)
func (s *singleObjectSerializer) Finish(writer io.Writer) error {
	s.Reset()
	return nil
}

// Reset resets the serializer to its initial state
func (s *singleObjectSerializer) Reset() {
	s.messageCount = 0
}
