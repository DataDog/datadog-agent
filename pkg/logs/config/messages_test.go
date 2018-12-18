// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMessages(t *testing.T) {
	messages := NewMessages()

	assert.Empty(t, messages.GetMessages())
	assert.Empty(t, messages.GetWarnings())
}

func TestAddMessageAndAddWarning(t *testing.T) {
	messages := NewMessages()

	messages.AddMessage("foo", "foo_message")
	assert.Equal(t, []string{"foo_message"}, messages.GetMessages())
	assert.Empty(t, messages.GetWarnings())

	messages.AddWarning("foo", "foo_warning")
	assert.Equal(t, []string{"foo_message"}, messages.GetMessages())
	assert.Equal(t, []string{"foo_warning"}, messages.GetWarnings())

	messages.AddMessage("bar", "bar_message")
	assert.ElementsMatch(t, []string{"foo_message", "bar_message"}, messages.GetMessages())
	assert.Equal(t, []string{"foo_warning"}, messages.GetWarnings())
	messages.AddWarning("bar", "bar_warning")
	assert.ElementsMatch(t, []string{"foo_message", "bar_message"}, messages.GetMessages())
	assert.ElementsMatch(t, []string{"foo_warning", "bar_warning"}, messages.GetWarnings())

	messages.AddMessage("bar", "bar_message_updated")
	assert.ElementsMatch(t, []string{"foo_message", "bar_message_updated"}, messages.GetMessages())
	assert.ElementsMatch(t, []string{"foo_warning", "bar_warning"}, messages.GetWarnings())
}

func TestRemoveMessageAndRemoveWarning(t *testing.T) {
	messages := NewMessages()

	messages.RemoveMessage("foo")
	assert.Empty(t, messages.GetMessages())
	assert.Empty(t, messages.GetWarnings())

	messages.AddMessage("foo", "foo_message")
	messages.AddMessage("bar", "bar_message")
	messages.AddWarning("foo", "foo_warning")
	messages.AddWarning("bar", "bar_warning")

	messages.RemoveMessage("foo")
	assert.Equal(t, []string{"bar_message"}, messages.GetMessages())
	assert.ElementsMatch(t, []string{"foo_warning", "bar_warning"}, messages.GetWarnings())
	messages.RemoveMessage("foo")
	assert.Equal(t, []string{"bar_message"}, messages.GetMessages())
	messages.RemoveWarning("bar")
	assert.Equal(t, []string{"foo_warning"}, messages.GetWarnings())
}
