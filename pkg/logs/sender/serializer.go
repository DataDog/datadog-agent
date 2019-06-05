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

var (
	// LineSerializer is a shared line formatter.
	LineSerializer Serializer = &lineSerializer{}
	// ArraySerializer is a shared array formatter.
	ArraySerializer Serializer = &arraySerializer{}
)

// lineSerializer transforms a message array into a payload
// separating content by new line character.
type lineSerializer struct{}

// Serialize concatenates all messages using
// a new line characater as a separator,
// for example:
// "{"message":"content1"}", "{"message":"content2"}"
// returns, "{"message":"content1"}\n{"message":"content2"}"
func (f *lineSerializer) Serialize(messages []*message.Message) []byte {
	buffer := bytes.NewBuffer(nil)
	for _, message := range messages {
		buffer.Write(message.Content)
		buffer.WriteByte('\n')
	}
	return buffer.Bytes()
}

// arraySerializer transforms a message array into a array string payload.
type arraySerializer struct{}

// Serialize transforms all messages into a array string
// for example:
// "{"message":"content1"}", "{"message":"content2"}"
// returns, "[{"message":"content1"},{"message":"content2"}]"
func (f *arraySerializer) Serialize(messages []*message.Message) []byte {
	buffer := bytes.NewBuffer(nil)
	buffer.WriteByte('[')
	for i, message := range messages {
		if i > 0 {
			buffer.WriteByte(',')
		}
		buffer.Write(message.Content)
	}
	buffer.WriteByte(']')
	return buffer.Bytes()
}
