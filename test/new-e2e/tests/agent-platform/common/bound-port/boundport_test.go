// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// Package boundport provides utilies for getting bound port information
package boundport

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromSS(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []BoundPort
	}{
		{
			name:  "single process on port 22",
			input: `LISTEN 0      4096               *:22              *:*    users:(("sshd",pid=726,fd=3))`,
			expected: []BoundPort{
				&boundPort{
					localAddress: "*",
					localPort:    22,
					processName:  "sshd",
					pid:          726,
				},
			},
		},
		{
			name:  "multiple processes on port 22",
			input: `LISTEN 0      4096               *:22              *:*    users:(("sshd",pid=726,fd=3),("systemd",pid=1,fd=118))`,
			expected: []BoundPort{
				&boundPort{
					localAddress: "*",
					localPort:    22,
					processName:  "sshd",
					pid:          726,
				},
				&boundPort{
					localAddress: "*",
					localPort:    22,
					processName:  "systemd",
					pid:          1,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := FromSs(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, len(tc.expected), len(res), "expected and result length should be equal")
			for i := range tc.expected {
				assert.Equal(t, tc.expected[i].LocalPort(), res[i].LocalPort())
				assert.Equal(t, tc.expected[i].LocalAddress(), res[i].LocalAddress())
				assert.Equal(t, tc.expected[i].Process(), res[i].Process())
				assert.Equal(t, tc.expected[i].PID(), res[i].PID())
			}
		})
	}
}
