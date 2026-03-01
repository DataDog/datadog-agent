// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package socket

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const testFrameSize = 4096

func recvMsg(t *testing.T, ch <-chan *message.Message) *message.Message {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
		return nil
	}
}

// ---------------------------------------------------------------------------
// Unstructured format tests
// ---------------------------------------------------------------------------

func TestStreamTailer_Unstructured_BasicMessages(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test", &config.LogsConfig{})
	outputChan := make(chan *message.Message, 10)

	tailer := NewStreamTailer(source, serverConn, outputChan, "", testFrameSize, 0, "")
	tailer.Start()

	clientConn.Write([]byte("foo\n"))
	msg := recvMsg(t, outputChan)
	assert.Equal(t, "foo", string(msg.GetContent()))

	clientConn.Write([]byte("bar\nboo\n"))
	msg = recvMsg(t, outputChan)
	assert.Equal(t, "bar", string(msg.GetContent()))
	msg = recvMsg(t, outputChan)
	assert.Equal(t, "boo", string(msg.GetContent()))

	clientConn.Close()
	tailer.Stop()
}

func TestStreamTailer_Unstructured_ConnectionCloseCleansUp(t *testing.T) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test", &config.LogsConfig{})
	outputChan := make(chan *message.Message, 10)

	tailer := NewStreamTailer(source, serverConn, outputChan, "", testFrameSize, 0, "")
	tailer.Start()

	// Close client side, tailer should stop gracefully.
	clientConn.Close()

	// Give the tailer a moment to observe EOF and shut down.
	time.Sleep(100 * time.Millisecond)
	tailer.Stop()
}

func TestStreamTailer_OnDoneCallbackFires(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test", &config.LogsConfig{})
	outputChan := make(chan *message.Message, 10)

	tailer := NewStreamTailer(source, serverConn, outputChan, "", testFrameSize, 0, "")

	done := make(chan struct{})
	tailer.SetOnDone(func() { close(done) })
	tailer.Start()

	clientConn.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("onDone callback was not invoked after connection close")
	}

	tailer.Stop()
}

func TestStreamTailer_Unstructured_SourceHostTag(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	logsConfig := &config.LogsConfig{
		Tags: []string{"test:tag"},
	}
	source := sources.NewLogSource("test-source", logsConfig)
	outputChan := make(chan *message.Message, 10)

	tailer := NewStreamTailer(source, serverConn, outputChan, "", testFrameSize, 0, "192.168.1.100")
	tailer.Start()

	clientConn.Write([]byte("foo\n"))
	msg := recvMsg(t, outputChan)
	assert.Contains(t, msg.Origin.Tags(nil), "source_host:192.168.1.100")

	clientConn.Close()
	tailer.Stop()
}

func TestStreamTailer_Unstructured_SourceHostTagFlagDisabled(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.BindEnvAndSetDefault("logs_config.use_sourcehost_tag", false)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	logsConfig := &config.LogsConfig{
		Tags: []string{"test:tag"},
	}
	source := sources.NewLogSource("test-source", logsConfig)
	outputChan := make(chan *message.Message, 10)

	// Even though sourceHostAddr is set, the tag should not appear when disabled.
	tailer := NewStreamTailer(source, serverConn, outputChan, "", testFrameSize, 0, "192.168.1.100")
	tailer.Start()

	clientConn.Write([]byte("foo\n"))
	msg := recvMsg(t, outputChan)
	assert.NotContains(t, msg.Origin.Tags(nil), "source_host:192.168.1.100",
		"source_host tag should not be added when flag is disabled")

	clientConn.Close()
	tailer.Stop()
}

// ---------------------------------------------------------------------------
// Syslog format tests (migrated from syslog_stream_tailer_test.go)
// ---------------------------------------------------------------------------

func TestStreamTailer_Syslog_NonTransparent(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog", &config.LogsConfig{Format: config.SyslogFormat})
	outputChan := make(chan *message.Message, 10)

	tailer := NewStreamTailer(source, serverConn, outputChan, config.SyslogFormat, testFrameSize, 0, "")
	tailer.Start()

	clientConn.Write([]byte("<14>1 2003-10-11T22:14:15.003Z myhost myapp - - - Hello world\n"))
	clientConn.Write([]byte("<11>1 2003-10-11T22:14:16.003Z myhost otherapp - - - Error occurred\n"))

	msg := recvMsg(t, outputChan)
	assert.Equal(t, message.StateStructured, msg.State)
	assert.Equal(t, "Hello world", string(msg.GetContent()))
	assert.Equal(t, message.StatusInfo, msg.Status)

	msg = recvMsg(t, outputChan)
	assert.Equal(t, "Error occurred", string(msg.GetContent()))
	assert.Equal(t, message.StatusError, msg.Status)

	clientConn.Close()
	tailer.Stop()
}

func TestStreamTailer_Syslog_OctetCounted(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog", &config.LogsConfig{Format: config.SyslogFormat})
	outputChan := make(chan *message.Message, 10)

	tailer := NewStreamTailer(source, serverConn, outputChan, config.SyslogFormat, testFrameSize, 0, "")
	tailer.Start()

	syslogMsg := "<14>1 2003-10-11T22:14:15.003Z h app - - - Hi"
	frame := []byte(fmt.Sprintf("%d %s", len(syslogMsg), syslogMsg))
	clientConn.Write(frame)

	msg := recvMsg(t, outputChan)
	assert.Equal(t, message.StateStructured, msg.State)
	assert.Equal(t, "Hi", string(msg.GetContent()))

	clientConn.Close()
	tailer.Stop()
}

func TestStreamTailer_Syslog_NULFraming(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog", &config.LogsConfig{Format: config.SyslogFormat})
	outputChan := make(chan *message.Message, 10)

	tailer := NewStreamTailer(source, serverConn, outputChan, config.SyslogFormat, testFrameSize, 0, "")
	tailer.Start()

	clientConn.Write([]byte("<14>1 2003-10-11T22:14:15.003Z myhost myapp - - - NUL hello\x00"))
	clientConn.Write([]byte("<11>1 2003-10-11T22:14:16.003Z myhost otherapp - - - NUL world\x00"))

	msg := recvMsg(t, outputChan)
	assert.Equal(t, message.StateStructured, msg.State)
	assert.Equal(t, "NUL hello", string(msg.GetContent()))
	assert.Equal(t, message.StatusInfo, msg.Status)

	msg = recvMsg(t, outputChan)
	assert.Equal(t, "NUL world", string(msg.GetContent()))
	assert.Equal(t, message.StatusError, msg.Status)

	clientConn.Close()
	tailer.Stop()
}

func TestStreamTailer_Syslog_StructuredContentRendered(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog", &config.LogsConfig{Format: config.SyslogFormat})
	outputChan := make(chan *message.Message, 10)

	tailer := NewStreamTailer(source, serverConn, outputChan, config.SyslogFormat, testFrameSize, 0, "")
	tailer.Start()

	clientConn.Write([]byte("<165>1 2003-10-11T22:14:15.003Z myhost evntslog - ID47 - Test msg\n"))

	msg := recvMsg(t, outputChan)
	rendered, err := msg.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(rendered, &data)
	require.NoError(t, err)

	assert.Equal(t, "Test msg", data["message"])
	syslogMap, ok := data["syslog"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "myhost", syslogMap["hostname"])
	assert.Equal(t, "evntslog", syslogMap["appname"])

	clientConn.Close()
	tailer.Stop()
}

func TestStreamTailer_Syslog_SourceServiceOverride(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog", &config.LogsConfig{Format: config.SyslogFormat})
	outputChan := make(chan *message.Message, 10)

	tailer := NewStreamTailer(source, serverConn, outputChan, config.SyslogFormat, testFrameSize, 0, "")
	tailer.Start()

	clientConn.Write([]byte("<14>1 2003-10-11T22:14:15.003Z myhost myapp - - - hello\n"))

	msg := recvMsg(t, outputChan)
	assert.Equal(t, "myapp", msg.Origin.Source())
	assert.Equal(t, "myapp", msg.Origin.Service())

	clientConn.Close()
	tailer.Stop()
}
