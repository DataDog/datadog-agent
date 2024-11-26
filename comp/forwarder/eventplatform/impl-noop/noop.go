// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatformimpl contains the logic for the noop forwarding component
package eventplatformimpl

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type noopForwarder struct {
}

// SendEventPlatformEvent sends messages to the event platform intake.
// SendEventPlatformEvent will drop messages and return an error if the input channel is already full.
func (s *noopForwarder) SendEventPlatformEvent(*message.Message, string) error {
	return nil
}

// SendEventPlatformEventBlocking sends messages to the event platform intake.
// SendEventPlatformEventBlocking will block if the input channel is already full.
func (s *noopForwarder) SendEventPlatformEventBlocking(*message.Message, string) error {
	return nil
}

// Purge clears out all pipeline channels, returning a map of eventType to list of messages in that were removed from each channel
func (s *noopForwarder) Purge() map[string][]*message.Message {
	return map[string][]*message.Message{}
}

// NewComponent creates a new EventPlatformForwarder
func NewComponent() eventplatform.Component {
	return &noopForwarder{}
}
