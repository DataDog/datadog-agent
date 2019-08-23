// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package v1

import "testing"

func TestParseClientIP(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		expected string
	}{
		{
			name:     "valid ipv4",
			args:     "127.0.0.1:1337",
			expected: "127.0.0.1",
		},
		{
			name:     "ipv4 no port",
			args:     "127.0.0.1:",
			expected: "127.0.0.1",
		},
		{
			name:     "ipv6",
			args:     "[2001:db8:1f70::999:de8:7648:6e8]:1337",
			expected: "2001:db8:1f70::999:de8:7648:6e8",
		},
		{
			name:     "valid ipv6 localhost",
			args:     "[::1]:1337",
			expected: "::1",
		},
		{
			name:     "ipv6 no port",
			args:     "[::1]:",
			expected: "::1",
		},
		{
			name:     "localhost",
			args:     "localhost:1337",
			expected: "localhost",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseClientIP(tt.args); got != tt.expected {
				t.Errorf("parseClientIP() == %v, expected %v", got, tt.expected)
			}
		})
	}
}
