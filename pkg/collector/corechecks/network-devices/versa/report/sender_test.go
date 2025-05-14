// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package report

import (
	"strings"
	"testing"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		errMsg   string
	}{
		{"2000-01-01 00:00:00.0", 946684800000, ""},
		{"2000/01/01 00:00:00", 946684800000, ""},
		{"invalid timestamp", 0, "error parsing timestamp"},
	}

	for _, test := range tests {
		result, err := parseTimestamp(test.input)
		if err != nil {
			if test.errMsg == "" {
				t.Errorf("Unexpected error parsing timestamp %s: %v", test.input, err)
			} else if !strings.Contains(err.Error(), test.errMsg) {
				t.Errorf("Expected error message to contain %q, got %q", test.errMsg, err.Error())
			}
			continue
		}
		if result != test.expected {
			t.Errorf("Expected %2f, got %2f for input %s", test.expected, result, test.input)
		}
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		errMsg   string
	}{
		{"1B", 1, ""},
		{"10B", 10, ""},
		{"999.8B", 999.8, ""},
		{"50.12KiB", 50.12 * 1024, ""},
		{"100MiB", 100 * 1024 * 1024, ""},
		{"80.09GiB", 80.09 * float64(1<<30), ""},
		{"", 0, "error parsing size"},
		{"1.5ZKiB", 0, "error parsing size"},
		{"9ZB", 0, "error parsing size"},
		{"120.05KB", 120.05e3, ""},
		{"101.5GB", 101.5e9, ""},
		{"1.5TB", 1.5e12, ""},
		{"1.5PB", 1.5e15, ""},
	}

	for _, test := range tests {
		result, err := parseSize(test.input)
		if err != nil {
			if test.errMsg == "" {
				t.Errorf("Unexpected error parsing size %s: %v", test.input, err)
			} else if !strings.Contains(err.Error(), test.errMsg) {
				t.Errorf("Expected error message to contain %q, got %q", test.errMsg, err.Error())
			}
			continue
		}
		if result != test.expected {
			t.Errorf("Expected %2f, got %2f for input %s", test.expected, result, test.input)
		}
	}
}
