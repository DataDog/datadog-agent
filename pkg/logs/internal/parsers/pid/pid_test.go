// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pid

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractFromJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int32
	}{
		{
			name:     "JSON with pid field",
			input:    `{"pid": 1234, "message": "test"}`,
			expected: 1234,
		},
		{
			name:     "JSON with process_id field",
			input:    `{"process_id": 5678, "message": "test"}`,
			expected: 5678,
		},
		{
			name:     "JSON with processId field",
			input:    `{"processId": 9012, "message": "test"}`,
			expected: 9012,
		},
		{
			name:     "JSON with PID field (uppercase)",
			input:    `{"PID": 3456, "message": "test"}`,
			expected: 3456,
		},
		{
			name:     "JSON with ProcessID field",
			input:    `{"ProcessID": 7890, "message": "test"}`,
			expected: 7890,
		},
		{
			name:     "JSON with pid as string",
			input:    `{"pid": "2468", "message": "test"}`,
			expected: 2468,
		},
		{
			name:     "JSON without PID",
			input:    `{"message": "test", "level": "info"}`,
			expected: 0,
		},
		{
			name:     "Invalid JSON",
			input:    `{invalid json}`,
			expected: 0,
		},
		{
			name:     "JSON with float pid",
			input:    `{"pid": 1234.0, "message": "test"}`,
			expected: 1234,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFromJSON([]byte(tt.input))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFromSyslog(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int32
	}{
		{
			name:     "Syslog RFC3164 format with brackets and colon",
			input:    "Sep 12 14:38:14 host my-app[1234]: Starting application",
			expected: 1234,
		},
		{
			name:     "Syslog with brackets and space",
			input:    "kernel[5678] some kernel message",
			expected: 5678,
		},
		{
			name:     "Syslog with process name",
			input:    "<34>Sep 12 14:38:14 host sshd[9012]: Accepted connection",
			expected: 9012,
		},
		{
			name:     "No brackets",
			input:    "Sep 12 14:38:14 host my-app: message without pid",
			expected: 0,
		},
		{
			name:     "Empty brackets",
			input:    "my-app[]: message",
			expected: 0,
		},
		{
			name:     "Non-numeric in brackets",
			input:    "my-app[abc]: message",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFromSyslog([]byte(tt.input))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFromPatterns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int32
	}{
		{
			name:     "Pattern: pid=1234",
			input:    "Starting process with pid=1234",
			expected: 1234,
		},
		{
			name:     "Pattern: pid:5678",
			input:    "Process started pid:5678 successfully",
			expected: 5678,
		},
		{
			name:     "Pattern: [pid: 9012]",
			input:    "[pid: 9012] Application started",
			expected: 9012,
		},
		{
			name:     "Pattern: (pid 3456)",
			input:    "Task started (pid 3456) with args",
			expected: 3456,
		},
		{
			name:     "Pattern: pid 7890 at start",
			input:    "pid 7890 - worker started",
			expected: 7890,
		},
		{
			name:     "Pattern: PID=1111 (case insensitive)",
			input:    "Worker running with PID=1111",
			expected: 1111,
		},
		{
			name:     "Pattern: process_id=2222",
			input:    "Job executing with process_id=2222",
			expected: 2222,
		},
		{
			name:     "Pattern: process id:3333",
			input:    "Starting with process id:3333",
			expected: 3333,
		},
		{
			name:     "Pattern: ProcessID=4444",
			input:    "Spawned with ProcessID=4444",
			expected: 4444,
		},
		{
			name:     "No PID pattern",
			input:    "This is a log message without any process identifier",
			expected: 0,
		},
		{
			name:     "PID with no number",
			input:    "Starting with pid=",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFromPatterns([]byte(tt.input))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractPID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int32
	}{
		{
			name:     "JSON format takes precedence",
			input:    `{"pid": 1111, "message": "pid=9999"}`,
			expected: 1111,
		},
		{
			name:     "Syslog format",
			input:    "Sep 12 14:38:14 host app[2222]: message",
			expected: 2222,
		},
		{
			name:     "Pattern format",
			input:    "Worker started with pid=3333",
			expected: 3333,
		},
		{
			name:     "No PID anywhere",
			input:    "Just a regular log message",
			expected: 0,
		},
		{
			name:     "Empty message",
			input:    "",
			expected: 0,
		},
		{
			name:     "Multiple PIDs - first match wins",
			input:    "Process pid=1111 spawned child pid=2222",
			expected: 1111,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractPID([]byte(tt.input))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertToInt32(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int32
	}{
		{
			name:     "float64",
			input:    float64(1234),
			expected: 1234,
		},
		{
			name:     "int",
			input:    int(5678),
			expected: 5678,
		},
		{
			name:     "int32",
			input:    int32(9012),
			expected: 9012,
		},
		{
			name:     "int64",
			input:    int64(3456),
			expected: 3456,
		},
		{
			name:     "string number",
			input:    "7890",
			expected: 7890,
		},
		{
			name:     "string non-number",
			input:    "abc",
			expected: 0,
		},
		{
			name:     "nil",
			input:    nil,
			expected: 0,
		},
		{
			name:     "bool",
			input:    true,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToInt32(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
