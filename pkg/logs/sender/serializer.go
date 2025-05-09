// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var (
	// LineSerializer is a shared line serializer.
	LineSerializer Serializer = &lineSerializer{isFirstMessage: true}
	// ArraySerializer is a shared line serializer.
	ArraySerializer Serializer = &arraySerializer{isFirstMessage: true}
)

// Serializer transforms a batch of messages into a payload.
// It is the one rendering the messages (i.e. either directly using
// raw []byte data from unstructured messages or turning structured
// messages into []byte data).
type Serializer interface {
	Serialize(message *message.Message, writer io.Writer) error
	Finish(writer io.Writer) error
}

// lineSerializer transforms a message array into a payload
// separating content by new line character.
type lineSerializer struct {
	isFirstMessage bool
}

// Serialize concatenates all messages using
// a new line characater as a separator,
// for example:
// "{"message":"content1"}", "{"message":"content2"}"
func (s *lineSerializer) Serialize(message *message.Message, writer io.Writer) error {
	if !s.isFirstMessage {
		if _, err := writer.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	s.isFirstMessage = false
	_, err := writer.Write(message.GetContent())
	return err
}

// Finish is a no-op for line serializer
func (s *lineSerializer) Finish(_ io.Writer) error {
	s.isFirstMessage = true
	return nil
}

// arraySerializer transforms a message array into a array string payload.
type arraySerializer struct {
	isFirstMessage bool
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
	s.isFirstMessage = true
	return err
}
