// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package syslog

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

func mockSyslogAddrs(t *testing.T, paths ...string) {
	prevAddrs := syslogAddrs
	t.Cleanup(func() {
		syslogAddrs = prevAddrs
	})
	syslogAddrs = paths
}

func mockSyslogFakeAddrs(t *testing.T) {
	mockSyslogAddrs(t, "/some/fake/addr.sock")
}

func TestGetSyslogConnection_NilURI(t *testing.T) {
	mockSyslogFakeAddrs(t)

	_, err := getSyslogConnection(nil)
	require.Error(t, err)
}

func TestHeaderFormatter_OldSchool(t *testing.T) {
	// Facility 16, rfc=false
	formatter := HeaderFormatter(16, false)
	require.NotNil(t, formatter)

	resultStr := formatter(types.WarnLvl)

	// Priority = facility * 8 + severity = 16 * 8 + 4 = 132
	appName := filepath.Base(os.Args[0])
	pid := os.Getpid()
	expected := fmt.Sprintf("<%d>%s[%d]:", 132, appName, pid)
	assert.Equal(t, expected, resultStr)
}

func TestHeaderFormatter_RFC5424(t *testing.T) {
	// Facility 16, RFC 5424 format
	formatter := HeaderFormatter(16, true)
	require.NotNil(t, formatter)

	resultStr := formatter(types.ErrorLvl)

	// Priority = facility * 8 + severity = 16 * 8 + 3 = 131
	appName := filepath.Base(os.Args[0])
	pid := os.Getpid()
	expected := fmt.Sprintf("<%d>1 %s %d - -", 131, appName, pid)
	assert.Equal(t, expected, resultStr)
}

func TestHeaderFormatter_AllLogLevels(t *testing.T) {
	formatter := HeaderFormatter(10, true)

	testCases := []struct {
		level            types.LogLevel
		expectedSeverity int
	}{
		{types.TraceLvl, 7},
		{types.DebugLvl, 7},
		{types.InfoLvl, 6},
		{types.WarnLvl, 4},
		{types.ErrorLvl, 3},
		{types.CriticalLvl, 2},
		{types.Off, 7},
	}

	for _, tc := range testCases {
		t.Run(tc.level.String(), func(t *testing.T) {
			resultStr := formatter(tc.level)

			expectedPriority := 10*8 + tc.expectedSeverity
			assert.Contains(t, resultStr, fmt.Sprintf("<%d>", expectedPriority))
		})
	}
}

func TestGetSyslogConnection_UnsupportedScheme(t *testing.T) {
	uri, err := url.Parse("http://example.com")
	require.NoError(t, err)

	conn, err := getSyslogConnection(uri)
	// Unsupported schemes return (nil, nil) - no error is raised
	// This documents the current behavior
	assert.Nil(t, conn)
	require.NoError(t, err)
}

func TestReceiver_Write_NoConnection(t *testing.T) {
	mockSyslogFakeAddrs(t)

	receiver := &Receiver{
		conn: nil,
		uri:  nil,
	}

	// Should try to reconnect but fail with nil URI
	_, err := receiver.Write([]byte("test message"))
	require.Error(t, err)
}

func TestReceiver_Write_WithMockConnection(t *testing.T) {
	// Create a mock connection using a pipe
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	receiver := &Receiver{
		conn: client,
	}

	// Start a goroutine to read from the server side
	received := make(chan string, 1)
	go func() {
		buf := make([]byte, 1024)
		n, err := server.Read(buf)
		if err == nil {
			received <- string(buf[:n])
		}
	}()

	message := "test syslog message"
	_, err := receiver.Write([]byte(message))
	require.NoError(t, err)

	// Verify the message was sent (with timeout)
	select {
	case msg := <-received:
		assert.Equal(t, message, msg)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Timeout waiting for message")
	}
}

func TestNewReceiver_InvalidURI(t *testing.T) {
	_, err := NewReceiver("://invalid-uri")
	require.ErrorContains(t, err, "bad syslog receiver configuration")
}

func TestNewReceiver_ValidURI_UDP(t *testing.T) {
	// Use a URI that won't actually connect but has valid format
	receiver, err := NewReceiver("udp://localhost:9999")
	require.NoError(t, err, "NewReceiver should succeed even if connection fails")

	// Should have parsed the URI even if connection failed
	require.NotNil(t, receiver.uri, "URI should be parsed")
	assert.Equal(t, "udp", receiver.uri.Scheme)
	assert.Equal(t, "localhost:9999", receiver.uri.Host)
}

func TestNewReceiver_ValidURI_UDP_WithServer(t *testing.T) {
	// Create a real UDP server for proper test isolation
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)

	conn, err := net.ListenUDP("udp", addr)
	require.NoError(t, err)
	defer conn.Close()

	// Get the actual port assigned
	port := conn.LocalAddr().(*net.UDPAddr).Port

	receiver, err := NewReceiver(fmt.Sprintf("udp://127.0.0.1:%d", port))
	require.NoError(t, err)
	require.NotNil(t, receiver.conn)
	require.NotNil(t, receiver.uri)
	assert.Equal(t, "udp", receiver.uri.Scheme)

	// Test sending a message to the server
	received := make(chan string, 1)
	go func() {
		buf := make([]byte, 1024)
		n, _, readErr := conn.ReadFromUDP(buf)
		if readErr == nil {
			received <- string(buf[:n])
		}
	}()

	message := "test message to udp server"
	_, err = receiver.Write([]byte(message))
	require.NoError(t, err)

	// Verify the message was received
	select {
	case msg := <-received:
		assert.Equal(t, message, msg)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Timeout waiting for UDP message")
	}
}

func TestNewReceiver_ValidURI_TCP(t *testing.T) {
	// Use a URI that won't actually connect but has valid format
	receiver, err := NewReceiver("tcp://127.0.0.1:514")
	require.NoError(t, err, "NewReceiver should succeed even if connection fails")

	// Should have parsed the URI
	require.NotNil(t, receiver.uri, "URI should be parsed")
	assert.Equal(t, "tcp", receiver.uri.Scheme)
	assert.Equal(t, "127.0.0.1:514", receiver.uri.Host)
}

func TestNewReceiver_ValidURI_TCP_WithServer(t *testing.T) {
	// Create a real TCP server for proper test isolation
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	// Get the actual port assigned
	port := listener.Addr().(*net.TCPAddr).Port

	// Accept connections in background
	received := make(chan string, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, readErr := conn.Read(buf)
		if readErr == nil {
			received <- string(buf[:n])
		}
	}()

	receiver, err := NewReceiver(fmt.Sprintf("tcp://127.0.0.1:%d", port))
	require.NoError(t, err)
	require.NotNil(t, receiver.conn)
	require.NotNil(t, receiver.uri)
	assert.Equal(t, "tcp", receiver.uri.Scheme)

	// Test sending a message to the server
	message := "test message to tcp server"
	_, err = receiver.Write([]byte(message))
	require.NoError(t, err)

	// Verify the message was received
	select {
	case msg := <-received:
		assert.Equal(t, message, msg)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Timeout waiting for TCP message")
	}
}

func TestNewReceiver_ValidURI_Unix(t *testing.T) {
	// Use a URI that won't actually connect but has valid format
	receiver, err := NewReceiver("unix:///some/fake/path")
	require.NoError(t, err, "NewReceiver should succeed even if connection fails")

	// Should have parsed the URI
	require.NotNil(t, receiver.uri, "URI should be parsed")
	assert.Equal(t, "unix", receiver.uri.Scheme)
	assert.Equal(t, "/some/fake/path", receiver.uri.Path)
}

func TestNewReceiver_ValidURI_Unix_WithSocket(t *testing.T) {
	// Create a real Unix socket for proper test isolation
	sockPath := filepath.Join(os.TempDir(), "test.sock")

	listener, err := net.Listen("unix", sockPath)
	require.NoError(t, err)
	defer listener.Close()
	defer os.Remove(sockPath)

	// Accept connections in background
	received := make(chan string, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, readErr := conn.Read(buf)
		if readErr == nil {
			received <- string(buf[:n])
		}
	}()

	receiver, err := NewReceiver("unix://" + sockPath)
	require.NoError(t, err)
	require.NotNil(t, receiver.conn)
	require.NotNil(t, receiver.uri)
	assert.Equal(t, "unix", receiver.uri.Scheme)

	// Test sending a message to the socket
	message := "test message to unix socket"
	_, err = receiver.Write([]byte(message))
	require.NoError(t, err)

	// Verify the message was received
	select {
	case msg := <-received:
		assert.Equal(t, message, msg)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Timeout waiting for Unix socket message")
	}
}

func TestReceiver_Flush(t *testing.T) {
	receiver := &Receiver{}

	// Flush should be a no-op and not panic
	assert.NotPanics(t, func() {
		receiver.Flush()
	})
}

func TestReceiver_Close(t *testing.T) {
	receiver := &Receiver{}

	err := receiver.Close()
	require.NoError(t, err)
}

func TestReceiver_Close_WithConnection(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	receiver := &Receiver{
		conn: client,
	}

	err := receiver.Close()
	require.NoError(t, err)

	// Note: Current implementation doesn't actually close the connection
	// This test documents the current behavior
}

func TestGetSyslogConnection_UnixScheme(t *testing.T) {
	// Create a temporary socket file path (won't actually exist)
	tempPath := filepath.Join(t.TempDir(), "test.sock")

	uri, err := url.Parse("unix://" + tempPath)
	require.NoError(t, err)

	conn, err := getSyslogConnection(uri)

	// Expected to fail since socket doesn't exist
	require.Error(t, err)
	assert.Nil(t, conn)
}

func TestGetSyslogConnection_UnixgramScheme(t *testing.T) {
	// Create a temporary socket file path
	tempPath := filepath.Join(t.TempDir(), "test.sock")

	uri, err := url.Parse("unixgram://" + tempPath)
	require.NoError(t, err)

	conn, err := getSyslogConnection(uri)

	// Expected to fail since socket doesn't exist
	require.Error(t, err)
	assert.Nil(t, conn)
}

// TestReceiver_Write_Reconnect tests the reconnection logic
func TestReceiver_Write_Reconnect(t *testing.T) {
	mockSyslogFakeAddrs(t)

	// Create a mock connection that will fail on write
	server1, client1 := net.Pipe()
	server1.Close() // Close server side to cause write to fail

	receiver := &Receiver{
		conn: client1,
		uri:  nil, // nil URI will cause reconnect to fail
	}

	message := "test message"
	_, err := receiver.Write([]byte(message))

	// Should fail to reconnect with nil URI
	require.Error(t, err)
}

func TestHeaderFormatter_EdgeCaseFacilities(t *testing.T) {
	testCases := []struct {
		name     string
		facility int
		level    types.LogLevel
	}{
		{"Facility 0", 0, types.InfoLvl},
		{"Facility 23", 23, types.InfoLvl},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			formatter := HeaderFormatter(tc.facility, false)
			require.NotNil(t, formatter)

			resultStr := formatter(tc.level)

			severity := levelToSyslogSeverity[tc.level]
			expectedPriority := tc.facility*8 + severity
			assert.Contains(t, resultStr, fmt.Sprintf("<%d>", expectedPriority))
		})
	}
}

func TestReceiver_Write_ThenReconnect(t *testing.T) {
	mockSyslogFakeAddrs(t)

	// First create a working connection
	server1, client1 := net.Pipe()

	receiver := &Receiver{
		conn: client1,
	}

	// Successful write
	received1 := make(chan string, 1)
	go func() {
		buf := make([]byte, 1024)
		n, _ := server1.Read(buf)
		received1 <- string(buf[:n])
	}()

	message1 := "first message"
	_, err := receiver.Write([]byte(message1))
	require.NoError(t, err)

	select {
	case msg := <-received1:
		assert.Equal(t, message1, msg)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Timeout waiting for first message")
	}

	// Now close the connection to trigger reconnect on next write
	server1.Close()
	client1.Close()

	// Try to send another message - should fail to reconnect
	message2 := "second message"
	_, err = receiver.Write([]byte(message2))
	require.Error(t, err) // Should fail because uri is nil
}
