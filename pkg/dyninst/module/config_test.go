// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTraceAgentURL(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected string
	}{
		{
			name:     "default",
			env:      map[string]string{},
			expected: "http://localhost:8126",
		},
		{
			name:     "agent host set",
			env:      map[string]string{agentHostEnvVar: "my-host"},
			expected: "http://my-host:8126",
		},
		{
			name:     "trace agent port set",
			env:      map[string]string{traceAgentPortEnvVar: "1234"},
			expected: "http://localhost:1234",
		},
		{
			name: "host and port set",
			env: map[string]string{
				agentHostEnvVar:      "my-host",
				traceAgentPortEnvVar: "1234",
			},
			expected: "http://my-host:1234",
		},
		{
			name:     "trace agent url set",
			env:      map[string]string{traceAgentURLEnvVar: "http://my-url:5678"},
			expected: "http://my-url:5678",
		},
		{
			name:     "trace agent url set with https",
			env:      map[string]string{traceAgentURLEnvVar: "https://my-url:5678"},
			expected: "https://my-url:5678",
		},
		{
			name:     "trace agent url set with unix socket",
			env:      map[string]string{traceAgentURLEnvVar: "unix:///var/run/datadog/apm.socket"},
			expected: "unix:///var/run/datadog/apm.socket",
		},
		{
			name: "trace agent url has precedence",
			env: map[string]string{
				traceAgentURLEnvVar:  "http://my-url:5678",
				agentHostEnvVar:      "my-host",
				traceAgentPortEnvVar: "1234",
			},
			expected: "http://my-url:5678",
		},
		{
			name: "invalid trace agent url",
			env: map[string]string{
				traceAgentURLEnvVar:  "not a url",
				agentHostEnvVar:      "my-host",
				traceAgentPortEnvVar: "1234",
			},
			expected: "http://my-host:1234",
		},
		{
			name:     "invalid trace agent url and no fallbacks",
			env:      map[string]string{traceAgentURLEnvVar: "not a url"},
			expected: "http://localhost:8126",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getEnv := func(key string) string {
				return tt.env[key]
			}
			actualURL := getTraceAgentURL(getEnv)
			assert.Equal(t, tt.expected, actualURL.String())
		})
	}
}
