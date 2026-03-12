// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package splite

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigArgs(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected []string
	}{
		{
			name:   "socket only",
			config: Config{Socket: "/path.sock"},
			expected: []string{
				"system-probe-lite", "run", "--socket", "/path.sock",
			},
		},
		{
			name: "all fields populated",
			config: Config{
				Socket:   "/var/run/sysprobe.sock",
				LogLevel: "debug",
				LogFile:  "/var/log/splite.log",
				PIDFile:  "/var/run/splite.pid",
			},
			expected: []string{
				"system-probe-lite", "run",
				"--socket", "/var/run/sysprobe.sock",
				"--log-level", "debug",
				"--log-file", "/var/log/splite.log",
				"--pid", "/var/run/splite.pid",
			},
		},
		{
			name: "socket and pid only",
			config: Config{
				Socket:  "/path.sock",
				PIDFile: "/var/run/splite.pid",
			},
			expected: []string{
				"system-probe-lite", "run",
				"--socket", "/path.sock",
				"--pid", "/var/run/splite.pid",
			},
		},
		{
			name: "socket with log level and log file",
			config: Config{
				Socket:   "/path.sock",
				LogLevel: "info",
				LogFile:  "/var/log/splite.log",
			},
			expected: []string{
				"system-probe-lite", "run",
				"--socket", "/path.sock",
				"--log-level", "info",
				"--log-file", "/var/log/splite.log",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.config.Args())
		})
	}
}
