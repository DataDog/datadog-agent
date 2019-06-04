// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Formatter transforms a batch of messages into a payload.
type Formatter interface {
	Format(messages []*message.Message) []byte
}

var (
	// LineFormatter is a shared line formatter.
	LineFormatter Formatter = &lineFormatter{}
	// ArrayFormatter is a shared array formatter.
	ArrayFormatter Formatter = &arrayFormatter{}
)

// lineFormatter transforms a message array into a payload
// separating content by new line character.
type lineFormatter struct{}

// Format concatenates all messages using
// a new line characater as a separator,
// for example:
// "{"message":"content1"}", "{"message":"content2"}"
// returns, "{"message":"content1"}\n{"message":"content2"}"
func (f *lineFormatter) Format(messages []*message.Message) []byte {
	buffer := bytes.NewBuffer(nil)
	for _, message := range messages {
		buffer.Write(message.Content)
		buffer.WriteByte('\n')
	}
	return buffer.Bytes()
}

// arrayFormatter transforms a message array into a array string payload.
type arrayFormatter struct{}

// Format transforms all messages into a array string
// for example:
// "{"message":"content1"}", "{"message":"content2"}"
// returns, "[{"message":"content1"},{"message":"content2"}]"
func (f *arrayFormatter) Format(messages []*message.Message) []byte {
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
