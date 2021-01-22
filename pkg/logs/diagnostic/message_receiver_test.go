// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package diagnostic

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
)

func TestEnableDisable(t *testing.T) {
	b := NewBufferedMessageReceiver()
	assert.True(t, b.SetEnabled(true))
	assert.False(t, b.SetEnabled(true))

	for i := 0; i < 10; i++ {
		b.HandleMessage(newMessage("", "", ""))
	}

	msg, ok := b.Next(nil)
	assert.True(t, ok)
	assert.NotEqual(t, "", msg)

	b.SetEnabled(false)

	// buffered messages should be cleared
	msg, ok = b.Next(nil)
	assert.False(t, ok)
	assert.Equal(t, "", msg)

	for i := 0; i < 10; i++ {
		b.HandleMessage(newMessage("", "", ""))
	}

	// disabled, no messages should have been buffered
	msg, ok = b.Next(nil)
	assert.False(t, ok)
	assert.Equal(t, "", msg)
}

func TestFilterAll(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test1", "a", "b"))
		b.HandleMessage(newMessage("test1", "1", "2"))
		b.HandleMessage(newMessage("test2", "a", "b"))
	}

	filters := Filters{
		Name:   "test1",
		Type:   "a",
		Source: "b",
	}

	for i := 0; i < 5; i++ {
		msg, ok := b.Next(&filters)
		assert.True(t, ok)
		assert.NotEqual(t, "", msg)
	}

	// Should be out of messages - have found 5 matches out of 10
	_, ok := b.Next(&filters)
	assert.False(t, ok)
}

func TestFilterTypeAndSource(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test", "a", "b"))
		b.HandleMessage(newMessage("test", "1", "2"))
	}

	filters := Filters{
		Type:   "a",
		Source: "b",
	}

	for i := 0; i < 5; i++ {
		msg, ok := b.Next(&filters)
		assert.True(t, ok)
		assert.NotEqual(t, "", msg)
	}

	// Should be out of messages - have found 5 matches out of 10
	_, ok := b.Next(&filters)
	assert.False(t, ok)
}

func TestFilterName(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test1", "a", "b"))
		b.HandleMessage(newMessage("test2", "a", "2"))
		b.HandleMessage(newMessage("test2", "b", "2"))
	}

	filters := Filters{
		Name: "test2",
	}

	for i := 0; i < 10; i++ {
		msg, ok := b.Next(&filters)
		assert.True(t, ok)
		assert.NotEqual(t, "", msg)
	}

	// Should be out of messages - have found 10 matches out of 15
	_, ok := b.Next(&filters)
	assert.False(t, ok)
}

func TestFilterSource(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test", "a", "b"))
		b.HandleMessage(newMessage("test", "a", "2"))
		b.HandleMessage(newMessage("test", "b", "2"))
	}

	filters := Filters{
		Source: "2",
	}

	for i := 0; i < 10; i++ {
		msg, ok := b.Next(&filters)
		assert.True(t, ok)
		assert.NotEqual(t, "", msg)
	}

	// Should be out of messages - have found 10 matches out of 15
	_, ok := b.Next(&filters)
	assert.False(t, ok)
}

func TestFilterType(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test", "a", "b"))
		b.HandleMessage(newMessage("test", "a", "2"))
		b.HandleMessage(newMessage("test", "b", "2"))
	}

	filters := Filters{
		Type: "a",
	}

	for i := 0; i < 10; i++ {
		msg, ok := b.Next(&filters)
		assert.True(t, ok)
		assert.NotEqual(t, "", msg)
	}

	// Should be out of messages - have found 10 matches out of 15
	_, ok := b.Next(&filters)
	assert.False(t, ok)
}

func TestNoFilters(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test", "a", "b"))
		b.HandleMessage(newMessage("test", "a", "2"))
		b.HandleMessage(newMessage("test", "b", "2"))
	}

	filters := Filters{
		Type:   "",
		Source: "",
	}

	for i := 0; i < 15; i++ {
		msg, ok := b.Next(&filters)
		assert.True(t, ok)
		assert.NotEqual(t, "", msg)
	}

	// Should be out of messages - have found 15 matches out of 15
	_, ok := b.Next(&filters)
	assert.False(t, ok)
}

func newMessage(n string, t string, s string) message.Message {
	cfg := &config.LogsConfig{
		Type:   t,
		Source: s,
	}
	source := config.NewLogSource(n, cfg)
	origin := message.NewOrigin(source)
	return *message.NewMessage([]byte("a"), origin, "", 0)
}
