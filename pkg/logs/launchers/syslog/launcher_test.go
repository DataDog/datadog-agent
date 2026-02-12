// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// freePort returns an available port for testing.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// mockPipelineProvider implements pipeline.Provider for testing.
type mockPipelineProvider struct {
	ch chan *message.Message
}

func (m *mockPipelineProvider) NextPipelineChan() chan *message.Message {
	return m.ch
}

func (m *mockPipelineProvider) GetOutputChan() chan *message.Message {
	return m.ch
}

func (m *mockPipelineProvider) NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor) {
	return m.ch, nil
}

func (m *mockPipelineProvider) Start()                  {}
func (m *mockPipelineProvider) Stop()                   {}
func (m *mockPipelineProvider) Flush(_ context.Context) {}

func TestTCPListener_EndToEnd(t *testing.T) {
	port := freePort(t)
	outputChan := make(chan *message.Message, 10)
	pp := &mockPipelineProvider{ch: outputChan}

	source := sources.NewLogSource("test-syslog-tcp", &config.LogsConfig{
		Type:     config.SyslogType,
		Port:     port,
		Protocol: "tcp",
	})

	listener := NewTCPListener(pp, source)
	listener.Start()
	defer listener.Stop()

	// Allow the listener to start accepting connections
	time.Sleep(100 * time.Millisecond)

	// Connect as a TCP client
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	defer conn.Close()

	// Send a syslog message with non-transparent framing
	_, err = conn.Write([]byte("<14>1 2003-10-11T22:14:15.003Z myhost myapp - - - TCP syslog test\n"))
	require.NoError(t, err)

	var msg *message.Message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for TCP syslog message")
	}

	assert.Equal(t, message.StateStructured, msg.State)
	assert.Equal(t, "TCP syslog test", string(msg.GetContent()))
	assert.Equal(t, message.StatusInfo, msg.Status)
}

func TestTCPListener_StructuredContent(t *testing.T) {
	port := freePort(t)
	outputChan := make(chan *message.Message, 10)
	pp := &mockPipelineProvider{ch: outputChan}

	source := sources.NewLogSource("test-syslog-tcp", &config.LogsConfig{
		Type:     config.SyslogType,
		Port:     port,
		Protocol: "tcp",
	})

	listener := NewTCPListener(pp, source)
	listener.Start()
	defer listener.Stop()

	time.Sleep(100 * time.Millisecond)

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("<165>1 2003-10-11T22:14:15.003Z myhost evntslog - ID47 - Structured test\n"))
	require.NoError(t, err)

	var msg *message.Message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	rendered, err := msg.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(rendered, &data)
	require.NoError(t, err)

	assert.Equal(t, "Structured test", data["message"])
	syslogMap, ok := data["syslog"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "myhost", syslogMap["hostname"])
	assert.Equal(t, "evntslog", syslogMap["appname"])
}

func TestUDPListener_EndToEnd(t *testing.T) {
	port := freePort(t)
	outputChan := make(chan *message.Message, 10)
	pp := &mockPipelineProvider{ch: outputChan}

	source := sources.NewLogSource("test-syslog-udp", &config.LogsConfig{
		Type:     config.SyslogType,
		Port:     port,
		Protocol: "udp",
	})

	listener := NewUDPListener(pp, source)
	listener.Start()
	defer listener.Stop()

	// Allow the listener to start
	time.Sleep(100 * time.Millisecond)

	// Send a UDP datagram
	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.Write([]byte("<14>1 2003-10-11T22:14:15.003Z myhost myapp - - - UDP syslog test"))
	require.NoError(t, err)

	var msg *message.Message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for UDP syslog message")
	}

	assert.Equal(t, message.StateStructured, msg.State)
	assert.Equal(t, "UDP syslog test", string(msg.GetContent()))
	assert.Equal(t, message.StatusInfo, msg.Status)
}

func TestTCPListener_MultipleConnections(t *testing.T) {
	port := freePort(t)
	outputChan := make(chan *message.Message, 20)
	pp := &mockPipelineProvider{ch: outputChan}

	source := sources.NewLogSource("test-syslog-tcp", &config.LogsConfig{
		Type:     config.SyslogType,
		Port:     port,
		Protocol: "tcp",
	})

	listener := NewTCPListener(pp, source)
	listener.Start()
	defer listener.Stop()

	time.Sleep(100 * time.Millisecond)

	// Open two concurrent TCP connections
	conn1, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	defer conn1.Close()

	conn2, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	defer conn2.Close()

	// Send from both connections
	_, err = conn1.Write([]byte("<14>1 2003-10-11T22:14:15.003Z h app - - - Conn1\n"))
	require.NoError(t, err)
	_, err = conn2.Write([]byte("<14>1 2003-10-11T22:14:15.003Z h app - - - Conn2\n"))
	require.NoError(t, err)

	messages := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case msg := <-outputChan:
			messages[string(msg.GetContent())] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for message %d", i+1)
		}
	}

	assert.True(t, messages["Conn1"], "missing Conn1 message")
	assert.True(t, messages["Conn2"], "missing Conn2 message")
}
