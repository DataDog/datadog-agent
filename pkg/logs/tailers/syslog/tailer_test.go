// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestTailer_EndToEnd_NonTransparent(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog", &config.LogsConfig{})
	outputChan := make(chan *message.Message, 10)

	tailer := NewTailer(source, outputChan, serverConn)
	tailer.Start()

	// Write two non-transparent framed syslog messages
	_, err := clientConn.Write([]byte("<14>1 2003-10-11T22:14:15.003Z myhost myapp - - - Hello world\n"))
	require.NoError(t, err)
	_, err = clientConn.Write([]byte("<11>1 2003-10-11T22:14:16.003Z myhost otherapp - - - Error occurred\n"))
	require.NoError(t, err)

	// Read the first message
	var msg *message.Message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for first message")
	}

	assert.Equal(t, message.StateStructured, msg.State)
	assert.Equal(t, "Hello world", string(msg.GetContent()))
	assert.Equal(t, message.StatusInfo, msg.Status) // severity 14%8=6 -> info

	// Read the second message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for second message")
	}

	assert.Equal(t, "Error occurred", string(msg.GetContent()))
	assert.Equal(t, message.StatusError, msg.Status) // severity 11%8=3 -> error

	// Close the client side to trigger EOF
	clientConn.Close()

	// Tailer should finish
	tailer.Stop()
}

func TestTailer_EndToEnd_OctetCounted(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog", &config.LogsConfig{})
	outputChan := make(chan *message.Message, 10)

	tailer := NewTailer(source, outputChan, serverConn)
	tailer.Start()

	// Write an octet-counted message: "48 <14>1 ... Hello"
	syslogMsg := "<14>1 2003-10-11T22:14:15.003Z h app - - - Hi"
	frame := []byte(fmt.Sprintf("%d %s", len(syslogMsg), syslogMsg))
	_, err := clientConn.Write(frame)
	require.NoError(t, err)

	var msg *message.Message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	assert.Equal(t, message.StateStructured, msg.State)
	assert.Equal(t, "Hi", string(msg.GetContent()))

	clientConn.Close()
	tailer.Stop()
}

func TestTailer_StructuredContentRendered(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog", &config.LogsConfig{})
	outputChan := make(chan *message.Message, 10)

	tailer := NewTailer(source, outputChan, serverConn)
	tailer.Start()

	_, err := clientConn.Write([]byte("<165>1 2003-10-11T22:14:15.003Z myhost evntslog - ID47 - Test msg\n"))
	require.NoError(t, err)

	var msg *message.Message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	// Render the structured content and verify it's valid JSON with syslog fields
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

// ---------------------------------------------------------------------------
// Reader-level NUL framing tests (RFC 6587 §3.4.2)
// ---------------------------------------------------------------------------

func TestReader_NULDelimitedFrames(t *testing.T) {
	// Two messages delimited by NUL instead of LF
	msg1 := "<14>1 2003-10-11T22:14:15.003Z myhost app - - - Hello"
	msg2 := "<11>1 2003-10-11T22:14:16.003Z myhost app - - - World"
	data := []byte(msg1 + "\x00" + msg2 + "\x00")

	reader := NewReader(bytes.NewReader(data))

	frame1, err := reader.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, msg1, string(frame1))

	frame2, err := reader.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, msg2, string(frame2))

	_, err = reader.ReadFrame()
	assert.Equal(t, io.EOF, err)
}

func TestReader_MixedNULAndLFFraming(t *testing.T) {
	// First message NUL-terminated, second LF-terminated, third NUL-terminated
	msg1 := "<14>1 2003-10-11T22:14:15.003Z h app - - - First"
	msg2 := "<14>1 2003-10-11T22:14:16.003Z h app - - - Second"
	msg3 := "<14>1 2003-10-11T22:14:17.003Z h app - - - Third"
	data := []byte(msg1 + "\x00" + msg2 + "\n" + msg3 + "\x00")

	reader := NewReader(bytes.NewReader(data))

	frame1, err := reader.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, msg1, string(frame1))

	frame2, err := reader.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, msg2, string(frame2))

	frame3, err := reader.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, msg3, string(frame3))

	_, err = reader.ReadFrame()
	assert.Equal(t, io.EOF, err)
}

func TestReader_StrayNULBetweenFrames(t *testing.T) {
	// Stray NUL bytes between two LF-terminated messages should be skipped
	msg1 := "<14>1 2003-10-11T22:14:15.003Z h app - - - Hello"
	msg2 := "<14>1 2003-10-11T22:14:16.003Z h app - - - World"
	data := []byte(msg1 + "\n\x00\x00\x00" + msg2 + "\n")

	reader := NewReader(bytes.NewReader(data))

	frame1, err := reader.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, msg1, string(frame1))

	frame2, err := reader.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, msg2, string(frame2))
}

func TestReader_NULInsideOctetCountedFrame(t *testing.T) {
	// NUL bytes inside an octet-counted frame are content, not delimiters.
	// The message contains a NUL byte in the body.
	syslogMsg := "<14>1 2003-10-11T22:14:15.003Z h app - - - Hello\x00World"
	frame := []byte(fmt.Sprintf("%d %s", len(syslogMsg), syslogMsg))

	reader := NewReader(bytes.NewReader(frame))

	result, err := reader.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, syslogMsg, string(result))
}

func TestReader_MixedOctetCountedAndNULFraming(t *testing.T) {
	// First message octet-counted, second NUL-terminated
	msg1 := "<14>1 2003-10-11T22:14:15.003Z h app - - - Octet"
	msg2 := "<14>1 2003-10-11T22:14:16.003Z h app - - - NulMsg"
	data := []byte(fmt.Sprintf("%d %s", len(msg1), msg1) + msg2 + "\x00")

	reader := NewReader(bytes.NewReader(data))

	frame1, err := reader.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, msg1, string(frame1))

	frame2, err := reader.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, msg2, string(frame2))
}

// ---------------------------------------------------------------------------
// End-to-end NUL framing through the full tailer pipeline
// ---------------------------------------------------------------------------

func TestTailer_EndToEnd_NULFraming(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	source := sources.NewLogSource("test-syslog", &config.LogsConfig{})
	outputChan := make(chan *message.Message, 10)

	tailer := NewTailer(source, outputChan, serverConn)
	tailer.Start()

	// Write two NUL-delimited syslog messages
	_, err := clientConn.Write([]byte("<14>1 2003-10-11T22:14:15.003Z myhost myapp - - - NUL hello\x00"))
	require.NoError(t, err)
	_, err = clientConn.Write([]byte("<11>1 2003-10-11T22:14:16.003Z myhost otherapp - - - NUL world\x00"))
	require.NoError(t, err)

	var msg *message.Message
	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for first NUL-framed message")
	}
	assert.Equal(t, message.StateStructured, msg.State)
	assert.Equal(t, "NUL hello", string(msg.GetContent()))
	assert.Equal(t, message.StatusInfo, msg.Status)

	select {
	case msg = <-outputChan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for second NUL-framed message")
	}
	assert.Equal(t, "NUL world", string(msg.GetContent()))
	assert.Equal(t, message.StatusError, msg.Status)

	clientConn.Close()
	tailer.Stop()
}
