// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// Package boundport provides utilies for getting bound port information
package boundport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromNetstat(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []BoundPort
	}{
		{
			name:  "single process on port 22",
			input: "tcp6       0      0 :::22                   :::*                    LISTEN      1/systemd",
			expected: []BoundPort{
				newBoundPort("::", 22, "tcp6", "systemd", 1),
			},
		},
		{
			name: "multiple transports",
			input: "tcp        0      0 127.0.0.1:5000          0.0.0.0:*               LISTEN      1661400/agent\n" +
				"udp        0      0 127.0.0.1:8125          0.0.0.0:*                           1661400/agent",
			expected: []BoundPort{
				newBoundPort("127.0.0.1", 5000, "tcp", "agent", 1661400),
				newBoundPort("127.0.0.1", 8125, "udp", "agent", 1661400),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := FromNetstat(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			require.Equal(t, len(tc.expected), len(res), "expected and result length should be equal")
			for i := range tc.expected {
				assert.Equal(t, tc.expected[i].LocalPort(), res[i].LocalPort())
				assert.Equal(t, tc.expected[i].LocalAddress(), res[i].LocalAddress())
				assert.Equal(t, tc.expected[i].Transport(), res[i].Transport())
				assert.Equal(t, tc.expected[i].Process(), res[i].Process())
				assert.Equal(t, tc.expected[i].PID(), res[i].PID())
			}
		})
	}
}

func TestFromSS(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []BoundPort
	}{
		{
			name:  "single process on port 22",
			input: "tcp LISTEN 0      4096               *:22              *:*    users:((\"sshd\",pid=726,fd=3))",
			expected: []BoundPort{
				newBoundPort("*", 22, "tcp", "sshd", 726),
			},
		},
		{
			name:  "multiple processes on port 22",
			input: "tcp LISTEN 0      4096               *:22              *:*    users:((\"sshd\",pid=726,fd=3),(\"systemd\",pid=1,fd=118))",
			expected: []BoundPort{
				newBoundPort("*", 22, "tcp", "sshd", 726),
				newBoundPort("*", 22, "tcp", "systemd", 1),
			},
		},
		{
			name: "multiple transports",
			input: "tcp LISTEN 0      4096               *:22              *:*    users:((\"sshd\",pid=726,fd=3))\n" +
				"udp UNCONN 0      4096               127.0.0.1:8125    *:*    users:((\"agent\",pid=43482,fd=20))",
			expected: []BoundPort{
				newBoundPort("*", 22, "tcp", "sshd", 726),
				newBoundPort("127.0.0.1", 8125, "udp", "agent", 43482),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := FromSs(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			require.Equal(t, len(tc.expected), len(res), "expected and result length should be equal")
			for i := range tc.expected {
				assert.Equal(t, tc.expected[i].LocalPort(), res[i].LocalPort())
				assert.Equal(t, tc.expected[i].LocalAddress(), res[i].LocalAddress())
				assert.Equal(t, tc.expected[i].Transport(), res[i].Transport())
				assert.Equal(t, tc.expected[i].Process(), res[i].Process())
				assert.Equal(t, tc.expected[i].PID(), res[i].PID())
			}
		})
	}
}
