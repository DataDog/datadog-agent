// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog/types"
)

// setSyslogAddrs is a helper function that sets syslogAddrs for testing
// and registers a cleanup function to restore the original value
func setSyslogAddrs(t *testing.T, addrs []string) {
	t.Helper()
	originalAddrs := syslogAddrs
	syslogAddrs = addrs
	t.Cleanup(func() {
		syslogAddrs = originalAddrs
	})
}

// getTestPC returns a program counter for testing purposes
// This captures the caller's PC for use in slog.Record
func getTestPC() uintptr {
	var pcs [1]uintptr
	// Skip 2 frames: runtime.Callers itself and getTestPC
	runtime.Callers(2, pcs[:])
	return pcs[0]
}

// mockConn is a mock implementation of net.Conn for testing
type mockConn struct {
	buf       *bytes.Buffer
	closed    bool
	failWrite bool
}

func newMockConn() *mockConn {
	return &mockConn{
		buf: &bytes.Buffer{},
	}
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	return m.buf.Read(b)
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	if m.failWrite {
		return 0, io.ErrClosedPipe
	}
	return m.buf.Write(b)
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return &net.UnixAddr{Name: "local", Net: "unix"}
}

func (m *mockConn) RemoteAddr() net.Addr {
	return &net.UnixAddr{Name: "remote", Net: "unix"}
}

func (m *mockConn) SetDeadline(time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(time.Time) error {
	return nil
}

func TestNewReceiver(t *testing.T) {
	// Use empty list to prevent actual syslog connections in tests
	setSyslogAddrs(t, []string{})

	tests := []struct {
		name       string
		loggerName string
		jsonFormat bool
		syslogURI  string
		syslogRFC  bool
		wantErr    bool
	}{
		{
			name:       "text format without URI",
			loggerName: "test-agent",
			jsonFormat: false,
			syslogURI:  "",
			syslogRFC:  false,
			wantErr:    false,
		},
		{
			name:       "json format without URI",
			loggerName: "test-agent",
			jsonFormat: true,
			syslogURI:  "",
			syslogRFC:  false,
			wantErr:    false,
		},
		{
			name:       "with RFC 5424 format",
			loggerName: "test-agent",
			jsonFormat: false,
			syslogURI:  "",
			syslogRFC:  true,
			wantErr:    false,
		},
		{
			name:       "invalid URI",
			loggerName: "test-agent",
			jsonFormat: false,
			syslogURI:  "://invalid",
			syslogRFC:  false,
			wantErr:    true,
		},
		{
			name:       "valid unix URI format",
			loggerName: "test-agent",
			jsonFormat: false,
			syslogURI:  "unix:///this/does/not/exist",
			syslogRFC:  false,
			wantErr:    false,
		},
		{
			name:       "valid udp URI format",
			loggerName: "test-agent",
			jsonFormat: false,
			syslogURI:  "udp://localhost:514",
			syslogRFC:  false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receiver, err := NewReceiver(tt.loggerName, tt.jsonFormat, tt.syslogURI, tt.syslogRFC)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, receiver)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, receiver)
				assert.Equal(t, tt.loggerName, receiver.loggerName)
				assert.NotNil(t, receiver.formatMessage)
				// Note: writeHeader may be nil if connection failed but that's ok
				// since NewReceiver returns (receiver, nil) even on connection failure
			}
		})
	}
}

func TestReceiver_formatTextMessage(t *testing.T) {
	setSyslogAddrs(t, []string{})

	receiver, err := NewReceiver("test-agent", false, "", false)
	require.NoError(t, err)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", getTestPC())

	var buff bytes.Buffer
	receiver.formatTextMessage(&buff, record)

	output := buff.String()
	// Format: loggerName | level | (file:line in function) | message
	pattern := `^test-agent \| \w+ \| \(.+syslog_test\.go:\d+ in TestReceiver_formatTextMessage\) \| test message\n$`
	assert.Regexp(t, regexp.MustCompile(pattern), output)
}

func TestReceiver_formatTextMessageWithAttributes(t *testing.T) {
	setSyslogAddrs(t, []string{})

	receiver, err := NewReceiver("test-agent", false, "", false)
	require.NoError(t, err)

	record := slog.NewRecord(time.Now(), slog.LevelWarn, "warning message", getTestPC())
	record.AddAttrs(slog.String("key", "value"))
	record.AddAttrs(slog.Int("count", 42))

	var buff bytes.Buffer
	receiver.formatTextMessage(&buff, record)

	output := buff.String()
	// Format: loggerName | level | (file:line in function) | key:value,count:42 | message
	pattern := `^test-agent \| warn \| \(.+\) \| key:value,count:42 \| warning message\n$`
	assert.Regexp(t, regexp.MustCompile(pattern), output)
}

func TestReceiver_formatJSONMessage(t *testing.T) {
	setSyslogAddrs(t, []string{})

	receiver, err := NewReceiver("test-agent", true, "", false)
	require.NoError(t, err)

	record := slog.NewRecord(time.Now(), slog.LevelError, "error message", getTestPC())

	var buff bytes.Buffer
	receiver.formatJSONMessage(&buff, record)

	jsonStr := buff.String()
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &jsonOutput)
	require.NoError(t, err, "Output should be valid JSON: %s", jsonStr)

	// Assert on individual fields
	assert.Equal(t, "test-agent", jsonOutput["agent"])
	assert.Equal(t, "error", jsonOutput["level"])
	assert.Equal(t, "error message", jsonOutput["msg"])
	assert.Contains(t, jsonOutput["relfile"], "syslog_test.go")
	assert.NotEmpty(t, jsonOutput["line"])
}

func TestReceiver_formatJSONMessageWithAttributes(t *testing.T) {
	setSyslogAddrs(t, []string{})

	receiver, err := NewReceiver("test-agent", true, "", false)
	require.NoError(t, err)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "info message", getTestPC())
	record.AddAttrs(slog.String("key", "value"))
	record.AddAttrs(slog.Int("count", 42))

	var buff bytes.Buffer
	receiver.formatJSONMessage(&buff, record)

	jsonStr := buff.String()
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &jsonOutput)
	require.NoError(t, err, "Output should be valid JSON: %s", jsonStr)

	// Assert on individual fields
	assert.Equal(t, "test-agent", jsonOutput["agent"])
	assert.Equal(t, "info message", jsonOutput["msg"])
	assert.Equal(t, "value", jsonOutput["key"])
	assert.Equal(t, "42", jsonOutput["count"])
}

func TestReceiver_format(t *testing.T) {
	setSyslogAddrs(t, []string{})

	receiver, err := NewReceiver("test-agent", false, "", false)
	require.NoError(t, err)

	// Set up writeHeader since connection failed
	receiver.writeHeader = getHeaderFormatter(20, false)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", getTestPC())

	output := receiver.format(context.Background(), record)

	// Should contain syslog header and formatted message
	// Format: <priority>appname[pid]: loggerName | level | (file:line in function) | message
	// Note: format() adds newline internally from formatTextMessage
	pattern := `^<\d+>\S+\[\d+\]: test-agent \| \w+ \| \(.+syslog_test\.go:\d+ in \w+\) \| test message\n$`
	assert.Regexp(t, regexp.MustCompile(pattern), output)
}

func TestReceiver_Write(t *testing.T) {
	setSyslogAddrs(t, []string{})

	receiver, err := NewReceiver("test-agent", false, "", false)
	require.NoError(t, err)

	// Replace connection with mock
	mockConn := newMockConn()
	receiver.conn = mockConn

	message := []byte("test log message\n")
	n, err := receiver.Write(message)

	assert.NoError(t, err)
	assert.Equal(t, len(message), n)
	assert.Equal(t, string(message), mockConn.buf.String())
}

func TestReceiver_WriteWithReconnect(t *testing.T) {
	setSyslogAddrs(t, []string{})

	receiver, err := NewReceiver("test-agent", false, "", false)
	require.NoError(t, err)

	// Set up a mock connection that fails on write
	failingConn := newMockConn()
	failingConn.failWrite = true
	receiver.conn = failingConn

	message := []byte("test log message\n")

	// This will fail and trigger reconnection attempt
	// Since we can't mock the reconnection, we expect an error
	_, err = receiver.Write(message)
	assert.Error(t, err)

	// Should have attempted to close the old connection
	assert.True(t, failingConn.closed)
}

func TestReceiver_Close(t *testing.T) {
	setSyslogAddrs(t, []string{})

	receiver, err := NewReceiver("test-agent", false, "", false)
	require.NoError(t, err)

	mockConn := newMockConn()
	receiver.conn = mockConn

	err = receiver.Close()
	assert.NoError(t, err)
	assert.True(t, mockConn.closed)
}

func TestGetHeaderFormatter(t *testing.T) {
	// Get actual values that will be used in the formatter
	pid := os.Getpid()
	appName := filepath.Base(os.Args[0])

	tests := []struct {
		name         string
		facility     int
		syslogRFC    bool
		level        slog.Level
		wantPriority int
		wantPattern  string
	}{
		{
			name:         "RFC 5424 format info level",
			facility:     20,
			syslogRFC:    true,
			level:        slog.LevelInfo,
			wantPriority: 166, // 20*8 + 6 (info severity)
			wantPattern:  `^<166>1 %s %d - -$`,
		},
		{
			name:         "RFC 5424 format error level",
			facility:     20,
			syslogRFC:    true,
			level:        slog.LevelError,
			wantPriority: 163, // 20*8 + 3 (error severity)
			wantPattern:  `^<163>1 %s %d - -$`,
		},
		{
			name:         "old format info level",
			facility:     20,
			syslogRFC:    false,
			level:        slog.LevelInfo,
			wantPriority: 166,
			wantPattern:  `^<166>%s\[%d\]:$`,
		},
		{
			name:         "old format error level",
			facility:     20,
			syslogRFC:    false,
			level:        slog.LevelError,
			wantPriority: 163,
			wantPattern:  `^<163>%s\[%d\]:$`,
		},
		{
			name:         "invalid facility defaults to 20",
			facility:     -1,
			syslogRFC:    false,
			level:        slog.LevelInfo,
			wantPriority: 166,
			wantPattern:  `^<166>%s\[%d\]:$`,
		},
		{
			name:         "facility out of range defaults to 20",
			facility:     25,
			syslogRFC:    false,
			level:        slog.LevelInfo,
			wantPriority: 166,
			wantPattern:  `^<166>%s\[%d\]:$`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := getHeaderFormatter(tt.facility, tt.syslogRFC)
			require.NotNil(t, formatter)

			var buff bytes.Buffer
			formatter(&buff, tt.level)

			output := buff.String()

			// Build expected pattern with actual appName and PID
			expectedPattern := fmt.Sprintf(tt.wantPattern, regexp.QuoteMeta(appName), pid)
			assert.Regexp(t, regexp.MustCompile(expectedPattern), output,
				"Output should match expected format with app name %q and PID %d", appName, pid)
		})
	}
}

func TestGetHeaderFormatterOldFormat(t *testing.T) {
	formatter := getHeaderFormatter(20, false)

	var buff bytes.Buffer
	formatter(&buff, slog.LevelWarn)

	output := buff.String()

	// Old format: <priority>appname[pid]:
	pattern := `^<\d+>\S+\[\d+\]:$`
	assert.Regexp(t, regexp.MustCompile(pattern), output)
}

func TestGetSyslogConnection(t *testing.T) {
	t.Run("nil URI attempts local connection", func(t *testing.T) {
		// Set empty list to ensure connection fails in tests
		setSyslogAddrs(t, []string{})

		// This will fail in test environments since we have no valid addresses
		conn, err := getSyslogConnection(nil)
		if conn != nil {
			defer conn.Close()
		}
		// Should fail since we have no valid syslog addresses
		assert.Error(t, err)
	})

	t.Run("unix scheme with invalid path", func(t *testing.T) {
		uri, _ := url.Parse("unix:///invalid/path/that/does/not/exist")
		conn, err := getSyslogConnection(uri)
		assert.Error(t, err)
		assert.Nil(t, conn)
	})

	t.Run("unixgram scheme with invalid path", func(t *testing.T) {
		uri, _ := url.Parse("unixgram:///invalid/path/that/does/not/exist")
		conn, err := getSyslogConnection(uri)
		assert.Error(t, err)
		assert.Nil(t, conn)
	})
}

func TestReceiver_IntegrationWithHandler(t *testing.T) {
	setSyslogAddrs(t, []string{})

	receiver, err := NewReceiver("test-agent", false, "", false)
	require.NoError(t, err)

	// Replace connection with mock and set up writeHeader
	mockConn := newMockConn()
	receiver.conn = mockConn
	receiver.writeHeader = getHeaderFormatter(20, false)

	// Get handler and log a message
	handler := receiver.Handler()
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "integration test message", getTestPC())

	err = handler.Handle(context.Background(), record)
	assert.NoError(t, err)

	// Check that message was written to mock connection
	output := mockConn.buf.String()
	// Format: <priority>appname[pid]: loggerName | level | (file:line in function) | message
	pattern := `^<\d+>.+\[\d+\]: test-agent \| \w+ \| \(.+syslog_test\.go:\d+ in \w+\) \| integration test message\n$`
	assert.Regexp(t, regexp.MustCompile(pattern), output)
}

func TestReceiver_IntegrationJSONFormat(t *testing.T) {
	setSyslogAddrs(t, []string{})

	receiver, err := NewReceiver("test-agent", true, "", true)
	require.NoError(t, err)

	// Replace connection with mock and set up writeHeader
	mockConn := newMockConn()
	receiver.conn = mockConn
	receiver.writeHeader = getHeaderFormatter(20, true)

	// Get handler and log a message
	handler := receiver.Handler()
	record := slog.NewRecord(time.Now(), slog.LevelWarn, "json test message", getTestPC())
	record.AddAttrs(slog.String("user", "john"))

	err = handler.Handle(context.Background(), record)
	assert.NoError(t, err)

	// Check that JSON message was written
	output := mockConn.buf.String()

	// Extract JSON part (after syslog header)
	jsonStart := strings.Index(output, "{")
	require.Greater(t, jsonStart, -1, "Output should contain JSON")

	jsonPart := output[jsonStart:]
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(jsonPart), &jsonOutput)
	require.NoError(t, err, "Output should contain valid JSON: %s", jsonPart)

	assert.Equal(t, "test-agent", jsonOutput["agent"])
	assert.Equal(t, "json test message", jsonOutput["msg"])
	assert.Equal(t, "john", jsonOutput["user"])
}

func TestReceiver_DifferentLogLevels(t *testing.T) {
	setSyslogAddrs(t, []string{})

	tests := []types.LogLevel{
		types.DebugLvl,
		types.InfoLvl,
		types.WarnLvl,
		types.ErrorLvl,
	}

	for _, tt := range tests {
		t.Run(tt.String(), func(t *testing.T) {
			receiver, err := NewReceiver("test-agent", false, "", false)
			require.NoError(t, err)

			mockConn := newMockConn()
			receiver.conn = mockConn

			record := slog.NewRecord(time.Now(), slog.Level(tt), "test message", getTestPC())

			var buff bytes.Buffer
			receiver.formatTextMessage(&buff, record)

			output := buff.String()
			// Format: loggerName | level | (file:line in function) | message
			pattern := regexp.MustCompile(`^test-agent \| ` + regexp.QuoteMeta(tt.String()) + ` \| \(.+syslog_test\.go:\d+ in \w+\) \| test message\n$`)
			assert.Regexp(t, pattern, output)
		})
	}
}

func TestReceiver_LoggerNameCasing(t *testing.T) {
	setSyslogAddrs(t, []string{})

	tests := []struct {
		name       string
		loggerName string
		jsonFormat bool
		expected   string
	}{
		{
			name:       "uppercase logger name in JSON",
			loggerName: "CORE",
			jsonFormat: true,
			expected:   `"agent":"core"`,
		},
		{
			name:       "mixed case logger name in JSON",
			loggerName: "DataDog-Agent",
			jsonFormat: true,
			expected:   `"agent":"datadog-agent"`,
		},
		{
			name:       "logger name preserved in text",
			loggerName: "CORE",
			jsonFormat: false,
			expected:   "CORE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receiver, err := NewReceiver(tt.loggerName, tt.jsonFormat, "", false)
			require.NoError(t, err)

			record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", getTestPC())
			var buff bytes.Buffer

			if tt.jsonFormat {
				receiver.formatJSONMessage(&buff, record)
			} else {
				receiver.formatTextMessage(&buff, record)
			}

			output := buff.String()
			assert.Contains(t, output, tt.expected)
		})
	}
}
