// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Serializer transforms a batch of messages into a payload.
type Serializer interface {
	Serialize(messages []*message.Message) []byte
}

// serializer represents a generic serializer that holds a buffer.
type serializer struct {
	buffer bytes.Buffer
}

// lineSerializer transforms a message array into a payload
// separating content by new line character.
type lineSerializer struct {
	serializer
}

// NewLineSerializer returns a new line serializer.
func NewLineSerializer() Serializer {
	return &lineSerializer{}
}

// Serialize concatenates all messages using
// a new line characater as a separator,
// for example:
// "{"message":"content1"}", "{"message":"content2"}"
// returns, "{"message":"content1"}\n{"message":"content2"}"
func (s *lineSerializer) Serialize(messages []*message.Message) []byte {
	s.buffer.Reset()
	for i, message := range messages {
		if i > 0 {
			s.buffer.WriteByte('\n')
		}
		s.buffer.Write(message.Content)
	}
	return s.buffer.Bytes()
}

// arraySerializer transforms a message array into a array string payload.
type arraySerializer struct {
	serializer
}

// NewArraySerializer returns a new line serializer.
func NewArraySerializer() Serializer {
	return &arraySerializer{}
}

// Serialize transforms all messages into a array string
// for example:
// "{"message":"content1"}", "{"message":"content2"}"
// returns, "[{"message":"content1"},{"message":"content2"}]"
func (s *arraySerializer) Serialize(messages []*message.Message) []byte {
	s.buffer.Reset()
	s.buffer.WriteByte('[')
	for i, message := range messages {
		if i > 0 {
			s.buffer.WriteByte(',')
		}
		s.buffer.Write(message.Content)
	}
	s.buffer.WriteByte(']')
	return s.buffer.Bytes()
}
