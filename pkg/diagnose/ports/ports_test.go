// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ports

import "testing"

func TestFormatProcessName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"agent.exe", "agent"},
		{"AGENT.EXE", "agent"},
		{"agent", "agent"},
		{"agent\x00\x00\x00", "agent"},
		{"someprocess.exe\x00", "someprocess"},
	}

	for _, tc := range testCases {
		got := FormatProcessName(tc.input)
		if got != tc.expected {
			t.Errorf("formatProcessName(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}
