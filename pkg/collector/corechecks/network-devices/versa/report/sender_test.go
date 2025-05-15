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
		{
			description:     "Valid uptime, upper case",
			input:           "40 Years 299 Days 7 Hours 5 Minutes and 39 Seconds.",
			expectedYears:   40,
			expectedDays:    299,
			expectedHours:   7,
			expectedMinutes: 5,
			expectedSeconds: 39,
		},
		{
			description:     "Valid uptime",
			input:           "10 days 2 hours 3 minutes 4 seconds.",
			expectedDays:    10,
			expectedHours:   2,
			expectedMinutes: 3,
			expectedSeconds: 4,
		},
		{
			description:     "Valid uptime with years",
			input:           "5 years 278 days 16 hours 0 minutes 30 seconds.",
			expectedYears:   5,
			expectedDays:    278,
			expectedHours:   16,
			expectedMinutes: 0,
			expectedSeconds: 30,
		},
		{
			// TODO: do I like this? it's more flexible, but the
			// data coming from the API is likely wrong
			description:  "Uptime missing years, valid days",
			input:        " years 5 days",
			expectedDays: 5,
		},
		{
			description: "Invalid uptime, mixed letters/numbers",
			input:       "5x years",
			errMsg:      "invalid numeric value",
		},
		{
			description: "Invalid uptime, spelled out",
			input:       "five years",
			errMsg:      "invalid numeric value",
		},
		{
			description: "Invalid uptime, special characters",
			input:       "5! years",
			errMsg:      "invalid numeric value",
		},
		{
			description: "Invalid uptime, expect integers",
			input:       "5.5 years",
			errMsg:      "invalid numeric value",
		},
		{
			description: "Invalid uptime, negative number",
			input:       "-5 years",
			errMsg:      "invalid numeric value",
		},
		{
			description: "Invalid uptime, empty value",
			input:       " years",
			errMsg:      "no valid time components found",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			result, err := parseUptimeString(test.input)
			if err != nil {
				if test.errMsg == "" {
					t.Errorf("Unexpected error parsing uptime %s: %v", test.input, err)
				} else if !strings.Contains(err.Error(), test.errMsg) {
					t.Errorf("Expected error message to contain %q, got %q", test.errMsg, err.Error())
				}
				return
			}

			if test.errMsg != "" {
				t.Errorf("Expected error containing %q but got none", test.errMsg)
				return
			}

			result *= float64(time.Millisecond) * 10
			years, days, hours, minutes, seconds := extractDurationComponents(time.Duration(result))

			if years != test.expectedYears ||
				days != test.expectedDays ||
				hours != test.expectedHours ||
				minutes != test.expectedMinutes ||
				seconds != test.expectedSeconds {
				t.Errorf("Result mismatch for %q:\nExpected: %d years %d days %d hours %d minutes %d seconds\nGot: %d years %d days %d hours %d minutes %d seconds",
					test.input,
					test.expectedYears, test.expectedDays, test.expectedHours, test.expectedMinutes, test.expectedSeconds,
					years, days, hours, minutes, seconds)
			}
		})
	}
}

// extractDurationComponents converts a duration into its constituent parts.
func extractDurationComponents(d time.Duration) (years, days, hours, minutes, seconds int64) {
	years = int64(d / (365 * 24 * time.Hour))
	d -= time.Duration(years) * 365 * 24 * time.Hour
	days = int64(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours = int64(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	minutes = int64(d / time.Minute)
	d -= time.Duration(minutes) * time.Minute
	seconds = int64(d / time.Second)

	return years, days, hours, minutes, seconds
}
