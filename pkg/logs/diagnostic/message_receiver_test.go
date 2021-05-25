// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
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

	b.HandleMessage(newMessage("", "", ""), []byte("a"))

	done := make(chan struct{})
	defer close(done)
	lineChan := b.Filter(nil, done)
	msg := <-lineChan
	assert.NotEqual(t, "", msg)

	b.SetEnabled(false)

	select {
	case <-lineChan:
		// buffered messages should be cleared
		t.Fail()
	default:
	}

	b.HandleMessage(newMessage("", "", ""), []byte("a"))

	select {
	case <-lineChan:
		// disabled, no messages should have been buffered
		t.Fail()
	default:
	}
}

func TestFilterAll(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test1", "a", "b"), []byte("a"))
		b.HandleMessage(newMessage("test1", "1", "2"), []byte("a"))
		b.HandleMessage(newMessage("test2", "a", "b"), []byte("a"))
	}

	filters := Filters{
		Name:   "test1",
		Type:   "a",
		Source: "b",
	}
	readFilteredLines(t, b, &filters, 5)
}

func TestFilterTypeAndSource(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test", "a", "b"), []byte("a"))
		b.HandleMessage(newMessage("test", "1", "2"), []byte("a"))
	}

	filters := Filters{
		Type:   "a",
		Source: "b",
	}

	readFilteredLines(t, b, &filters, 5)
}

func TestFilterName(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test1", "a", "b"), []byte("a"))
		b.HandleMessage(newMessage("test2", "a", "2"), []byte("a"))
		b.HandleMessage(newMessage("test2", "b", "2"), []byte("a"))
	}

	filters := Filters{
		Name: "test2",
	}

	readFilteredLines(t, b, &filters, 10)
}

func TestFilterSource(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test", "a", "b"), []byte("a"))
		b.HandleMessage(newMessage("test", "a", "2"), []byte("a"))
		b.HandleMessage(newMessage("test", "b", "2"), []byte("a"))
	}

	filters := Filters{
		Source: "2",
	}

	readFilteredLines(t, b, &filters, 10)
}

func TestFilterType(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test", "a", "b"), []byte("a"))
		b.HandleMessage(newMessage("test", "a", "2"), []byte("a"))
		b.HandleMessage(newMessage("test", "b", "2"), []byte("a"))
	}

	filters := Filters{
		Type: "a",
	}

	readFilteredLines(t, b, &filters, 10)
}

func TestNoFilters(t *testing.T) {

	b := NewBufferedMessageReceiver()
	b.SetEnabled(true)

	for i := 0; i < 5; i++ {
		b.HandleMessage(newMessage("test", "a", "b"), []byte("a"))
		b.HandleMessage(newMessage("test", "a", "2"), []byte("a"))
		b.HandleMessage(newMessage("test", "b", "2"), []byte("a"))
	}

	filters := Filters{
		Type:   "",
		Source: "",
	}

	readFilteredLines(t, b, &filters, 15)
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

func readFilteredLines(t *testing.T, b *BufferedMessageReceiver, filters *Filters, expectedLineCount int) {
	done := make(chan struct{})
	defer close(done)
	lineChan := b.Filter(filters, done)

	for i := 0; i < expectedLineCount; i++ {
		msg := <-lineChan
		assert.NotEqual(t, "", msg)
	}

	select {
	case <-lineChan:
		// Should be out of messages
		t.Fail()
	default:
	}
}
