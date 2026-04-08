// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package socket

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// newTestUDPConn creates a UDP listener on a random port and returns the
// server conn (*net.UDPConn) and the address to send datagrams to.
func newTestUDPConn(t *testing.T) (*net.UDPConn, *net.UDPAddr) {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", addr)
	require.NoError(t, err)
	return conn, conn.LocalAddr().(*net.UDPAddr)
}

func TestDatagramTailer_Syslog_EndToEnd(t *testing.T) {
	serverConn, serverAddr := newTestUDPConn(t)
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog-udp", &logsConfig.LogsConfig{Format: logsConfig.SyslogFormat})
	outputChan := make(chan *message.Message, 10)

	tailer := NewDatagramTailer(source, serverConn, outputChan, true, 0)
	tailer.Start()

	clientConn, err := net.DialUDP("udp", nil, serverAddr)
	require.NoError(t, err)
	defer clientConn.Close()

	_, err = clientConn.Write([]byte("<14>1 2003-10-11T22:14:15.003Z myhost myapp - - - Hello UDP"))
	require.NoError(t, err)

	var msg *message.Message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for first UDP message")
	}

	assert.Equal(t, message.StateStructured, msg.State)
	assert.Equal(t, "Hello UDP", string(msg.GetContent()))
	assert.Equal(t, message.StatusInfo, msg.Status)

	_, err = clientConn.Write([]byte("<11>1 2003-10-11T22:14:16.003Z myhost otherapp - - - Error UDP"))
	require.NoError(t, err)

	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for second UDP message")
	}

	assert.Equal(t, "Error UDP", string(msg.GetContent()))
	assert.Equal(t, message.StatusError, msg.Status)

	tailer.Stop()
}

func TestDatagramTailer_Syslog_SourceHostTag(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.BindEnvAndSetDefault("logs_config.use_sourcehost_tag", true)

	serverConn, serverAddr := newTestUDPConn(t)
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog-udp", &logsConfig.LogsConfig{Format: logsConfig.SyslogFormat})
	outputChan := make(chan *message.Message, 10)

	tailer := NewDatagramTailer(source, serverConn, outputChan, true, 0)
	tailer.Start()

	clientConn, err := net.DialUDP("udp", nil, serverAddr)
	require.NoError(t, err)
	defer clientConn.Close()

	_, err = clientConn.Write([]byte("<14>1 2003-10-11T22:14:15.003Z h app - - - Tagged"))
	require.NoError(t, err)

	var msg *message.Message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	assert.Equal(t, "Tagged", string(msg.GetContent()))

	tags := msg.Origin.Tags(nil)
	found := false
	for _, tag := range tags {
		if len(tag) > len("source_host:") && tag[:len("source_host:")] == "source_host:" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected source_host tag, got tags: %v", tags)

	tailer.Stop()
}

func TestDatagramTailer_Syslog_SourceHostTagDisabled(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.BindEnvAndSetDefault("logs_config.use_sourcehost_tag", false)

	serverConn, serverAddr := newTestUDPConn(t)
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog-udp", &logsConfig.LogsConfig{Format: logsConfig.SyslogFormat})
	outputChan := make(chan *message.Message, 10)

	tailer := NewDatagramTailer(source, serverConn, outputChan, true, 0)
	tailer.Start()

	clientConn, err := net.DialUDP("udp", nil, serverAddr)
	require.NoError(t, err)
	defer clientConn.Close()

	_, err = clientConn.Write([]byte("<14>1 2003-10-11T22:14:15.003Z h app - - - NoTag"))
	require.NoError(t, err)

	var msg *message.Message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	assert.Equal(t, "NoTag", string(msg.GetContent()))

	tags := msg.Origin.Tags(nil)
	for _, tag := range tags {
		assert.False(t, len(tag) > len("source_host:") && tag[:len("source_host:")] == "source_host:",
			"unexpected source_host tag found: %s", tag)
	}

	tailer.Stop()
}

func TestDatagramTailer_Unstructured_EndToEnd(t *testing.T) {
	serverConn, serverAddr := newTestUDPConn(t)
	defer serverConn.Close()

	source := sources.NewLogSource("test-udp", &logsConfig.LogsConfig{})
	outputChan := make(chan *message.Message, 10)

	tailer := NewDatagramTailer(source, serverConn, outputChan, true, 0)
	tailer.Start()

	clientConn, err := net.DialUDP("udp", nil, serverAddr)
	require.NoError(t, err)
	defer clientConn.Close()

	_, err = clientConn.Write([]byte("plain text log line"))
	require.NoError(t, err)

	var msg *message.Message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for UDP message")
	}

	assert.Equal(t, "plain text log line", string(msg.GetContent()))
	assert.Equal(t, message.StateUnstructured, msg.State)

	tailer.Stop()
}
