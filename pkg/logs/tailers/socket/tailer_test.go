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
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
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

func TestSourceHostTag(t *testing.T) {
	msgChan := make(chan *message.Message)
	r, w := net.Pipe()
	logsConfig := &config.LogsConfig{
		Tags: []string{"test:tag"},
	}

	logSource := sources.NewLogSource("test-source", logsConfig)
	tailer := NewTailer(logSource, r, msgChan, readWithIP)
	tailer.Start()

	var msg *message.Message
	w.Write([]byte("foo\n"))
	msg = <-msgChan
	assert.Equal(t, []string{"source_host:192.168.1.100", "test:tag"}, msg.Tags())
	tailer.Stop()
}

func TestSourceHostTagFlagDisabled(t *testing.T) {
	mockConfig := configmock.New(t)
	// Set the config flag for source_host tag to false
	mockConfig.BindEnvAndSetDefault("logs_config.use_sourcehost_tag", false)

	// Set up test components
	msgChan := make(chan *message.Message)
	r, w := net.Pipe()
	logsConfig := &config.LogsConfig{
		Tags: []string{"test:tag"},
	}

	logSource := sources.NewLogSource("test-source", logsConfig)
	tailer := NewTailer(logSource, r, msgChan, readWithIP)
	tailer.Start()

	var msg *message.Message
	w.Write([]byte("foo\n"))
	msg = <-msgChan

	// Assert that only the original tag is present (source_host tag should not be added)
	assert.Equal(t, []string{"test:tag"}, msg.Tags(), "source_host tag should not be added when flag is disabled")

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

func readWithIP(tailer *Tailer) ([]byte, string, error) {
	inBuf := make([]byte, 4096)
	n, err := tailer.Conn.Read(inBuf)
	if err != nil {
		return nil, "", err
	}
	mockIPAddress := "192.168.1.100:8080"
	return inBuf[:n], mockIPAddress, nil
}
