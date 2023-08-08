// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/stretchr/testify/assert"
)

func TestCustomWriterUnbuffered(t *testing.T) {
	// Custom writer should pass-through buffering behaviour of target process
	testContent := []byte("log line\nlog line\n")
	config := &Config{
		channel:   make(chan *config.ChannelMessage, 2),
		isEnabled: true,
	}
	cw := &CustomWriter{
		LogConfig: config,
	}
	go cw.Write(testContent)
	numMessages := 0
	select {
	case message := <-config.channel:
		assert.Equal(t, []byte("log line\nlog line\n"), message.Content)
		numMessages++
	case <-time.After(100 * time.Millisecond):
		t.FailNow()
	}

	assert.Equal(t, 1, numMessages)
}

func TestCustomWriterShouldBuffer(t *testing.T) {
	// Custom writer should buffer log chunks not ending in a newline when isEnabled: true
	testContentChunk1 := []byte("this is")
	testContentChunk2 := []byte(" a log line\n")
	config := &Config{
		channel:   make(chan *config.ChannelMessage, 2),
		isEnabled: true,
	}
	cw := &CustomWriter{
		LogConfig:    config,
		LineBuffer:   bytes.Buffer{},
		ShouldBuffer: true,
	}
	go func() {
		cw.Write(testContentChunk1)
		cw.Write(testContentChunk2)
	}()

	numMessages := 0
	select {
	case message := <-config.channel:
		assert.Equal(t, []byte("this is a log line\n"), message.Content)
		numMessages++
	case <-time.After(100 * time.Millisecond):
		t.FailNow()
	}

	assert.Equal(t, 1, numMessages)
}

func TestCustomWriterShoudBufferOverflow(t *testing.T) {
	testMaxBufferSize := 5

	testContentChunk1 := []byte(strings.Repeat("a", testMaxBufferSize))
	testContentChunk2 := []byte("b\n")
	config := &Config{
		channel:   make(chan *config.ChannelMessage, 2),
		isEnabled: true,
	}
	cw := &CustomWriter{
		LogConfig:    config,
		LineBuffer:   bytes.Buffer{},
		ShouldBuffer: true,
	}

	go func() {
		var originalStdout = os.Stdout
		null, _ := os.Open(os.DevNull)
		os.Stdout = null
		cw.writeWithMaxBufferSize(testContentChunk1, testMaxBufferSize)
		cw.writeWithMaxBufferSize(testContentChunk2, testMaxBufferSize)
		os.Stdout = originalStdout
	}()

	var messages [][]byte

	for i := 0; i < 2; i++ {
		select {
		case message := <-config.channel:
			messages = append(messages, message.Content)
		case <-time.After(100 * time.Millisecond):
			t.FailNow()
		}
	}

	assert.Equal(t, 2, len(messages))
	assert.Equal(t, []byte(strings.Repeat("a", testMaxBufferSize)), messages[0])
	assert.Equal(t, []byte("b\n"), messages[1])
}

func TestCustomWriterMaxBufferSize(t *testing.T) {
	testMaxBufferSize := 5

	testContent := []byte(strings.Repeat("a", testMaxBufferSize+1))
	config := &Config{
		channel:   make(chan *config.ChannelMessage, 2),
		isEnabled: true,
	}
	cw := &CustomWriter{
		LogConfig: config,
	}
	go cw.writeWithMaxBufferSize(testContent, testMaxBufferSize)
	numMessages := 0
	select {
	case message := <-config.channel:
		assert.Equal(t, []byte(strings.Repeat("a", testMaxBufferSize)), message.Content)
		numMessages++
	case <-time.After(100 * time.Millisecond):
		t.FailNow()
	}

	assert.Equal(t, 1, numMessages)
}

func TestWriteEnabled(t *testing.T) {
	testContent := []byte("hello this is a log")
	logChannel := make(chan *config.ChannelMessage)
	config := &Config{
		channel:   logChannel,
		isEnabled: true,
	}
	go Write(config, testContent, false)
	select {
	case received := <-logChannel:
		assert.NotNil(t, received)
		assert.Equal(t, testContent, received.Content)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "We should have received logs")
	}
}

func TestWriteEnabledIsError(t *testing.T) {
	testContent := []byte("hello this is a log")
	logChannel := make(chan *config.ChannelMessage)
	config := &Config{
		channel:   logChannel,
		isEnabled: true,
	}
	go Write(config, testContent, true)
	select {
	case received := <-logChannel:
		assert.NotNil(t, received)
		assert.Equal(t, testContent, received.Content)
		assert.True(t, received.IsError)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "We should have received logs")
	}
}

func TestWriteDisabled(t *testing.T) {
	testContent := []byte("hello this is a log")
	logChannel := make(chan *config.ChannelMessage)
	config := &Config{
		channel:   logChannel,
		isEnabled: false,
	}
	go Write(config, testContent, false)
	select {
	case <-logChannel:
		assert.Fail(t, "We should not have received logs")
	case <-time.After(100 * time.Millisecond):
		assert.True(t, true)
	}
}

func TestCreateConfig(t *testing.T) {
	config := CreateConfig("fake-origin")
	assert.Equal(t, 5*time.Second, config.FlushTimeout)
	assert.Equal(t, "fake-origin", config.source)
	assert.Equal(t, "DD_LOG_AGENT", string(config.loggerName))
}

func TestCreateConfigWithSource(t *testing.T) {
	t.Setenv("DD_SOURCE", "python")
	config := CreateConfig("cloudrun")
	assert.Equal(t, 5*time.Second, config.FlushTimeout)
	assert.Equal(t, "python", config.source)
	assert.Equal(t, "DD_LOG_AGENT", string(config.loggerName))
}

func TestIsEnabledTrue(t *testing.T) {
	assert.True(t, isEnabled("True"))
	assert.True(t, isEnabled("TRUE"))
	assert.True(t, isEnabled("true"))
}

func TestIsEnabledFalse(t *testing.T) {
	assert.False(t, isEnabled(""))
	assert.False(t, isEnabled("false"))
	assert.False(t, isEnabled("1"))
	assert.False(t, isEnabled("FALSE"))
}
