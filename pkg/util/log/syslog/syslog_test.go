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
	"strings"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestCreateSyslogHeaderFormatter_DefaultParams(t *testing.T) {
	formatter := CreateSyslogHeaderFormatter("")
	require.NotNil(t, formatter)

	// Test with InfoLvl (severity 6)
	result := formatter("test message", seelog.InfoLvl, nil)
	resultStr := fmt.Sprintf("%v", result)

	// Default facility is 20
	// Priority = facility * 8 + severity = 20 * 8 + 6 = 166
	appName := filepath.Base(os.Args[0])
	pid := os.Getpid()
	expected := fmt.Sprintf("<%d>%s[%d]:", 166, appName, pid)
	assert.Equal(t, expected, resultStr)
}

func TestCreateSyslogHeaderFormatter_CustomFacilityOldSchool(t *testing.T) {
	// Facility 16, rfc=false
	formatter := CreateSyslogHeaderFormatter("16,false")
	require.NotNil(t, formatter)

	result := formatter("test message", seelog.WarnLvl, nil)
	resultStr := fmt.Sprintf("%v", result)

	// Priority = facility * 8 + severity = 16 * 8 + 4 = 132
	appName := filepath.Base(os.Args[0])
	pid := os.Getpid()
	expected := fmt.Sprintf("<%d>%s[%d]:", 132, appName, pid)
	assert.Equal(t, expected, resultStr)
}

func TestCreateSyslogHeaderFormatter_RFC5424(t *testing.T) {
	// Facility 16, RFC 5424 format
	formatter := CreateSyslogHeaderFormatter("16,true")
	require.NotNil(t, formatter)

	result := formatter("test message", seelog.ErrorLvl, nil)
	resultStr := fmt.Sprintf("%v", result)

	// Priority = facility * 8 + severity = 16 * 8 + 3 = 131
	appName := filepath.Base(os.Args[0])
	pid := os.Getpid()
	expected := fmt.Sprintf("<%d>1 %s %d - -", 131, appName, pid)
	assert.Equal(t, expected, resultStr)
}

func TestCreateSyslogHeaderFormatter_AllLogLevels(t *testing.T) {
	formatter := CreateSyslogHeaderFormatter("10,true")

	testCases := []struct {
		level            seelog.LogLevel
		expectedSeverity int
	}{
		{seelog.TraceLvl, 7},
		{seelog.DebugLvl, 7},
		{seelog.InfoLvl, 6},
		{seelog.WarnLvl, 4},
		{seelog.ErrorLvl, 3},
		{seelog.CriticalLvl, 2},
		{seelog.Off, 7},
	}

	for _, tc := range testCases {
		t.Run(tc.level.String(), func(t *testing.T) {
			result := formatter("test", tc.level, nil)
			resultStr := fmt.Sprintf("%v", result)

			expectedPriority := 10*8 + tc.expectedSeverity
			assert.Contains(t, resultStr, fmt.Sprintf("<%d>", expectedPriority))
		})
	}
}

func TestCreateSyslogHeaderFormatter_InvalidFacility(t *testing.T) {
	// Facility out of range should use default (20)
	formatter := CreateSyslogHeaderFormatter("999,false")
	require.NotNil(t, formatter)

	result := formatter("test", seelog.InfoLvl, nil)
	resultStr := fmt.Sprintf("%v", result)

	// Should use default facility 20
	// Priority = 20 * 8 + 6 = 166
	assert.Contains(t, resultStr, "<166>")
}

func TestCreateSyslogHeaderFormatter_NegativeFacility(t *testing.T) {
	// Negative facility should use default (20)
	formatter := CreateSyslogHeaderFormatter("-5,false")
	require.NotNil(t, formatter)

	result := formatter("test", seelog.InfoLvl, nil)
	resultStr := fmt.Sprintf("%v", result)

	// Should use default facility 20
	assert.Contains(t, resultStr, "<166>")
}

func TestCreateSyslogHeaderFormatter_BadlyFormattedParams(t *testing.T) {
	// Too many parameters
	formatter := CreateSyslogHeaderFormatter("10,true,extra")
	require.NotNil(t, formatter)

	result := formatter("test", seelog.InfoLvl, nil)
	resultStr := fmt.Sprintf("%v", result)

	// Should use defaults (facility 20, old-school format)
	assert.Contains(t, resultStr, "<166>")
	appName := filepath.Base(os.Args[0])
	assert.Contains(t, resultStr, appName+"[")
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

func TestReceiver_ReceiveMessage_Disabled(t *testing.T) {
	receiver := &Receiver{
		enabled: false,
	}

	err := receiver.ReceiveMessage("test message", seelog.InfoLvl, nil)
	require.NoError(t, err, "Disabled receiver should not return error")
}

func TestReceiver_ReceiveMessage_NoConnection(t *testing.T) {
	mockSyslogFakeAddrs(t)

	receiver := &Receiver{
		enabled: true,
		conn:    nil,
		uri:     nil,
	}

	// Should try to reconnect but fail with nil URI
	err := receiver.ReceiveMessage("test message", seelog.InfoLvl, nil)
	require.Error(t, err)
}

func TestReceiver_ReceiveMessage_WithMockConnection(t *testing.T) {
	// Create a mock connection using a pipe
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	receiver := &Receiver{
		enabled: true,
		conn:    client,
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
	err := receiver.ReceiveMessage(message, seelog.InfoLvl, nil)
	require.NoError(t, err)

	// Verify the message was sent (with timeout)
	select {
	case msg := <-received:
		assert.Equal(t, message, msg)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Timeout waiting for message")
	}
}

func TestReceiver_AfterParse_InvalidURI(t *testing.T) {
	receiver := &Receiver{}

	initArgs := seelog.CustomReceiverInitArgs{
		XmlCustomAttrs: map[string]string{
			"uri": "://invalid-uri",
		},
	}

	err := receiver.AfterParse(initArgs)
	require.ErrorContains(t, err, "bad syslog receiver configuration")
	assert.False(t, receiver.enabled)
}

func TestReceiver_AfterParse_ValidURI_UDP(t *testing.T) {
	receiver := &Receiver{}

	// Use a URI that won't actually connect but has valid format
	initArgs := seelog.CustomReceiverInitArgs{
		XmlCustomAttrs: map[string]string{
			"uri": "udp://localhost:9999",
		},
	}

	// AfterParse should succeed even if connection fails (it only prints the error)
	err := receiver.AfterParse(initArgs)
	require.NoError(t, err, "AfterParse should return nil even if connection fails")

	// Should have parsed the URI even if connection failed
	require.NotNil(t, receiver.uri, "URI should be parsed")
	assert.Equal(t, "udp", receiver.uri.Scheme)
	assert.Equal(t, "localhost:9999", receiver.uri.Host)
	assert.True(t, receiver.enabled)
}

func TestReceiver_AfterParse_ValidURI_UDP_WithServer(t *testing.T) {
	// Create a real UDP server for proper test isolation
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)

	conn, err := net.ListenUDP("udp", addr)
	require.NoError(t, err)
	defer conn.Close()

	// Get the actual port assigned
	port := conn.LocalAddr().(*net.UDPAddr).Port

	receiver := &Receiver{}
	initArgs := seelog.CustomReceiverInitArgs{
		XmlCustomAttrs: map[string]string{
			"uri": fmt.Sprintf("udp://127.0.0.1:%d", port),
		},
	}

	// Should successfully connect to our test server
	err = receiver.AfterParse(initArgs)
	require.NoError(t, err)
	assert.True(t, receiver.enabled)
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
	err = receiver.ReceiveMessage(message, seelog.InfoLvl, nil)
	require.NoError(t, err)

	// Verify the message was received
	select {
	case msg := <-received:
		assert.Equal(t, message, msg)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Timeout waiting for UDP message")
	}
}

func TestReceiver_AfterParse_ValidURI_TCP(t *testing.T) {
	receiver := &Receiver{}

	initArgs := seelog.CustomReceiverInitArgs{
		XmlCustomAttrs: map[string]string{
			"uri": "tcp://127.0.0.1:514",
		},
	}

	// AfterParse should succeed even if connection fails
	err := receiver.AfterParse(initArgs)
	require.NoError(t, err, "AfterParse should return nil even if connection fails")

	// Should have parsed the URI
	require.NotNil(t, receiver.uri, "URI should be parsed")
	assert.Equal(t, "tcp", receiver.uri.Scheme)
	assert.Equal(t, "127.0.0.1:514", receiver.uri.Host)
	assert.True(t, receiver.enabled)
}

func TestReceiver_AfterParse_ValidURI_TCP_WithServer(t *testing.T) {
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

	receiver := &Receiver{}
	initArgs := seelog.CustomReceiverInitArgs{
		XmlCustomAttrs: map[string]string{
			"uri": fmt.Sprintf("tcp://127.0.0.1:%d", port),
		},
	}

	// Should successfully connect to our test server
	err = receiver.AfterParse(initArgs)
	require.NoError(t, err)
	assert.True(t, receiver.enabled)
	require.NotNil(t, receiver.conn)
	require.NotNil(t, receiver.uri)
	assert.Equal(t, "tcp", receiver.uri.Scheme)

	// Test sending a message to the server
	message := "test message to tcp server"
	err = receiver.ReceiveMessage(message, seelog.InfoLvl, nil)
	require.NoError(t, err)

	// Verify the message was received
	select {
	case msg := <-received:
		assert.Equal(t, message, msg)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Timeout waiting for TCP message")
	}
}

func TestReceiver_AfterParse_ValidURI_Unix(t *testing.T) {
	receiver := &Receiver{}

	initArgs := seelog.CustomReceiverInitArgs{
		XmlCustomAttrs: map[string]string{
			"uri": "unix:///some/fake/path",
		},
	}

	// AfterParse should succeed even if connection fails
	err := receiver.AfterParse(initArgs)
	require.NoError(t, err, "AfterParse should return nil even if connection fails")

	// Should have parsed the URI
	require.NotNil(t, receiver.uri, "URI should be parsed")
	assert.Equal(t, "unix", receiver.uri.Scheme)
	assert.Equal(t, "/some/fake/path", receiver.uri.Path)
	assert.True(t, receiver.enabled)
}

func TestReceiver_AfterParse_ValidURI_Unix_WithSocket(t *testing.T) {
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

	receiver := &Receiver{}
	initArgs := seelog.CustomReceiverInitArgs{
		XmlCustomAttrs: map[string]string{
			"uri": "unix://" + sockPath,
		},
	}

	// Should successfully connect to our test socket
	err = receiver.AfterParse(initArgs)
	require.NoError(t, err)
	assert.True(t, receiver.enabled)
	require.NotNil(t, receiver.conn)
	require.NotNil(t, receiver.uri)
	assert.Equal(t, "unix", receiver.uri.Scheme)

	// Test sending a message to the socket
	message := "test message to unix socket"
	err = receiver.ReceiveMessage(message, seelog.InfoLvl, nil)
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
		enabled: true,
		conn:    client,
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

// TestReceiver_ReceiveMessage_Reconnect tests the reconnection logic
func TestReceiver_ReceiveMessage_Reconnect(t *testing.T) {
	mockSyslogFakeAddrs(t)

	// Create a mock connection that will fail on write
	server1, client1 := net.Pipe()
	server1.Close() // Close server side to cause write to fail

	receiver := &Receiver{
		enabled: true,
		conn:    client1,
		uri:     nil, // nil URI will cause reconnect to fail
	}

	message := "test message"
	err := receiver.ReceiveMessage(message, seelog.InfoLvl, nil)

	// Should fail to reconnect with nil URI
	require.Error(t, err)
}

func TestCreateSyslogHeaderFormatter_EdgeCaseFacilities(t *testing.T) {
	testCases := []struct {
		name     string
		params   string
		level    seelog.LogLevel
		facility int // expected facility
	}{
		{"Facility 0", "0,false", seelog.InfoLvl, 0},
		{"Facility 23", "23,false", seelog.InfoLvl, 23},
		{"Facility 24 (invalid)", "24,false", seelog.InfoLvl, 20}, // default
		{"Non-numeric", "abc,false", seelog.InfoLvl, 20},          // default
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			formatter := CreateSyslogHeaderFormatter(tc.params)
			require.NotNil(t, formatter)

			result := formatter("test", tc.level, nil)
			resultStr := fmt.Sprintf("%v", result)

			severity := levelToSyslogSeverity[tc.level]
			expectedPriority := tc.facility*8 + severity
			assert.Contains(t, resultStr, fmt.Sprintf("<%d>", expectedPriority))
		})
	}
}

func TestReceiver_ReceiveMessage_WriteThenReconnect(t *testing.T) {
	mockSyslogFakeAddrs(t)

	// First create a working connection
	server1, client1 := net.Pipe()

	receiver := &Receiver{
		enabled: true,
		conn:    client1,
	}

	// Successful write
	received1 := make(chan string, 1)
	go func() {
		buf := make([]byte, 1024)
		n, _ := server1.Read(buf)
		received1 <- string(buf[:n])
	}()

	message1 := "first message"
	err := receiver.ReceiveMessage(message1, seelog.InfoLvl, nil)
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
	err = receiver.ReceiveMessage(message2, seelog.InfoLvl, nil)
	require.Error(t, err) // Should fail because uri is nil
}

func TestCreateSyslogHeaderFormatter_RFCParameter(t *testing.T) {
	testCases := []struct {
		name            string
		params          string
		expectRFC       bool
		expectOldSchool bool
	}{
		{"RFC true", "10,true", true, false},
		{"RFC false", "10,false", false, true},
		{"RFC anything else", "10,maybe", false, true},
		{"RFC empty", "10,", false, true},
	}

	appName := filepath.Base(os.Args[0])
	pid := os.Getpid()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			formatter := CreateSyslogHeaderFormatter(tc.params)
			result := formatter("test", seelog.InfoLvl, nil)
			resultStr := fmt.Sprintf("%v", result)

			if tc.expectRFC {
				// RFC 5424 format: <PRI>1 APP PID - -
				assert.True(t, strings.Contains(resultStr, ">1 "), "Should contain RFC 5424 version")
				assert.Contains(t, resultStr, appName)
				assert.Contains(t, resultStr, fmt.Sprintf(" %d - -", pid))
			}

			if tc.expectOldSchool {
				// Old-school format: <PRI>APP[PID]:
				assert.Contains(t, resultStr, appName+"[")
				assert.Contains(t, resultStr, fmt.Sprintf("%d]:", pid))
			}
		})
	}
}
