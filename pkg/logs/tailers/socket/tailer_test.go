// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package socket

import (
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestReadAndForwardShouldSucceedWithSuccessfulRead(t *testing.T) {
	msgChan := make(chan *message.Message)
	r, w := net.Pipe()
	tailer := NewTailer(sources.NewLogSource("", &config.LogsConfig{}), r, msgChan, read)
	tailer.Start()

	var msg *message.Message

	// should receive and decode one message
	w.Write([]byte("foo\n"))
	msg = <-msgChan
	assert.Equal(t, "foo", string(msg.GetContent()))

	// should receive and decode two messages
	w.Write([]byte("bar\nboo\n"))
	msg = <-msgChan
	assert.Equal(t, "bar", string(msg.GetContent()))
	msg = <-msgChan
	assert.Equal(t, "boo", string(msg.GetContent()))

	tailer.Stop()
}

func TestReadShouldFailWithError(t *testing.T) {
	msgChan := make(chan *message.Message)
	r, w := net.Pipe()
	read := func(*Tailer) ([]byte, string, error) { return nil, "", errors.New("") }
	tailer := NewTailer(sources.NewLogSource("", &config.LogsConfig{}), r, msgChan, read)
	tailer.Start()

	w.Write([]byte("foo\n"))
	select {
	case <-msgChan:
		assert.Fail(t, "no data should return")
	default:
		break
	}

	tailer.Stop()
}

func TestDuplicateTags(t *testing.T) {
	msgChan := make(chan *message.Message)
	r, w := net.Pipe()
	// Create a log source with a sample configuration, if needed
	logSource := sources.NewLogSource("test-source", &config.LogsConfig{})

	// Define the read function to append tags to the message
	read := func(tailer *Tailer) ([]byte, string, error) {
		inBuf := make([]byte, 4096)
		n, err := tailer.Conn.Read(inBuf)
		if err != nil {
			return nil, "", err
		}
		// Append tags to the message based on your logic
		return inBuf[:n], "", nil
	}

	tailer := NewTailer(logSource, r, msgChan, read)
	tailer.Start()

	var msg *message.Message

	// should receive and decode one message
	w.Write([]byte("foo\n"))
	msg = <-msgChan
	// Adding tags to the message
	msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, "test_tag:tag1")
	assert.Equal(t, "foo", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "test_tag:tag1")

	// should receive and decode two messages
	w.Write([]byte("bar\nboo\n"))
	msg = <-msgChan
	msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, "test_tag:tag2")
	assert.Equal(t, "bar", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "test_tag:tag2")

	msg = <-msgChan
	msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, "test_tag:tag3")
	assert.Equal(t, "boo", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "test_tag:tag3")

	tailer.Stop()
}

func read(tailer *Tailer) ([]byte, string, error) {
	inBuf := make([]byte, 4096)
	n, err := tailer.Conn.Read(inBuf)
	if err != nil {
		return nil, "", err
	}
	return inBuf[:n], "", nil
}
