// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMessages(t *testing.T) {
	messages := NewMessages()

	assert.Empty(t, messages.GetMessages())
}

func TestAddMessage(t *testing.T) {
	messages := NewMessages()

	messages.AddMessage("foo", "foo_message")
	assert.Equal(t, []string{"foo_message"}, messages.GetMessages())

	messages.AddMessage("bar", "bar_message")
	assert.ElementsMatch(t, []string{"foo_message", "bar_message"}, messages.GetMessages())

	messages.AddMessage("bar", "bar_message_updated")
	assert.ElementsMatch(t, []string{"foo_message", "bar_message_updated"}, messages.GetMessages())
}

func TestRemoveMessage(t *testing.T) {
	messages := NewMessages()

	messages.RemoveMessage("foo")
	assert.Empty(t, messages.GetMessages())

	messages.AddMessage("foo", "foo_message")
	messages.AddMessage("bar", "bar_message")

	messages.RemoveMessage("foo")
	assert.Equal(t, []string{"bar_message"}, messages.GetMessages())

	messages.RemoveMessage("foo")
	assert.Equal(t, []string{"bar_message"}, messages.GetMessages())
}
