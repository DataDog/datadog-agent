// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var (
	// LineSerializer is a shared line serializer.
	LineSerializer Serializer = &lineSerializer{}
	// ArraySerializer is a shared line serializer.
	ArraySerializer Serializer = &arraySerializer{}
)

// Serializer transforms a batch of messages into a payload.
// It is the one rendering the messages (i.e. either directly using
// raw []byte data from unstructured messages or turning structured
// messages into []byte data).
type Serializer interface {
	Serialize(messages []*message.Message) []byte
}

// lineSerializer transforms a message array into a payload
// separating content by new line character.
type lineSerializer struct{}

// Serialize concatenates all messages using
// a new line characater as a separator,
// for example:
// "{"message":"content1"}", "{"message":"content2"}"
// returns, "{"message":"content1"}\n{"message":"content2"}"
func (s *lineSerializer) Serialize(messages []*message.Message) []byte {
	var buffer bytes.Buffer
	for i, message := range messages {
		if i > 0 {
			buffer.WriteByte('\n')
		}
		buffer.Write(message.GetContent())
	}
	return buffer.Bytes()
}

// arraySerializer transforms a message array into a array string payload.
type arraySerializer struct{}

// Serialize transforms all messages into a array string
// for example:
// "{"message":"content1"}", "{"message":"content2"}"
// returns, "[{"message":"content1"},{"message":"content2"}]"
func (s *arraySerializer) Serialize(messages []*message.Message) []byte {
	var buffer bytes.Buffer
	buffer.WriteByte('[')
	for i, message := range messages {
		if i > 0 {
			buffer.WriteByte(',')
		}
		buffer.Write(message.GetContent())
	}
	buffer.WriteByte(']')
	return buffer.Bytes()
}
