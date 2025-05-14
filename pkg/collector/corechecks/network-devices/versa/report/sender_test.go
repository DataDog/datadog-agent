// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package report

import (
	"strings"
	"testing"
	"time"
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

func TestParseUptimeString(t *testing.T) {
	tests := []struct {
		description     string
		input           string
		expectedYears   int64
		expectedDays    int64
		expectedHours   int64
		expectedMinutes int64
		expectedSeconds int64
		errMsg          string
	}{
		{"Valid uptime", "10 days 2 hours 3 minutes 4 seconds.", 0, 10, 2, 3, 4, ""},
		{"Valid uptime with years", "5 years 278 days 16 hours 0 minutes 30 seconds.", 5, 278, 16, 0, 30, ""},
		{"Invalid uptime", "5x years, invalid", 0, 0, 0, 0, 0, ""},
	}

	for _, test := range tests {
		result, err := parseUptimeString(test.input)
		if err != nil {
			if test.errMsg == "" {
				t.Errorf("Unexpected error parsing uptime %s: %v", test.input, err)
			} else if !strings.Contains(err.Error(), test.errMsg) {
				t.Errorf("Expected error message to contain %q, got %q", test.errMsg, err.Error())
			}
			continue
		}

		result *= float64(time.Millisecond) * 10
		resultDuration := time.Duration(result)
		years := resultDuration / (365 * 24 * time.Hour)
		resultDuration -= years * 365 * 24 * time.Hour
		days := resultDuration / (24 * time.Hour)
		resultDuration -= days * 24 * time.Hour
		hours := resultDuration / time.Hour
		resultDuration -= hours * time.Hour
		minutes := resultDuration / time.Minute
		resultDuration -= minutes * time.Minute
		seconds := resultDuration / time.Second
		resultDuration -= seconds * time.Second
		if int64(years) != test.expectedYears || int64(days) != test.expectedDays || int64(hours) != test.expectedHours || int64(minutes) != test.expectedMinutes || int64(seconds) != test.expectedSeconds {
			t.Errorf("Result mismatch, expected %s, got %d years %d days %d hours %d minutes %d seconds", test.input, years, days, hours, minutes, seconds)
		}
	}
}
