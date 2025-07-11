// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func FuzzParseKubernetes(f *testing.F) {
	// Add seed corpus based on real Kubernetes log formats
	timestamps := []string{
		"2018-09-20T11:54:11.753589172Z",
		"2018-09-20T11:54:11Z",
		"2018-09-20T11:54:11.123Z",
		"2018-09-20T11:54:11.123456789Z",
		"2023-12-25T23:59:59.999999999Z",
	}

	streams := []string{"stdout", "stderr"}
	flags := []string{"F", "P"}

	// Valid messages with all combinations
	for _, ts := range timestamps {
		for _, stream := range streams {
			for _, flag := range flags {
				// Normal message
				f.Add([]byte(fmt.Sprintf("%s %s %s This is a log message", ts, stream, flag)))

				// Empty content
				f.Add([]byte(fmt.Sprintf("%s %s %s", ts, stream, flag)))

				// Content with special characters
				f.Add([]byte(fmt.Sprintf("%s %s %s Special chars: \n\t\r", ts, stream, flag)))

				// Multi-word content
				f.Add([]byte(fmt.Sprintf("%s %s %s word1 word2 word3", ts, stream, flag)))
			}
		}
	}

	// Edge cases
	f.Add([]byte(""))                                      // Empty input
	f.Add([]byte("2018-09-20T11:54:11.753589172Z"))        // Only timestamp
	f.Add([]byte("2018-09-20T11:54:11.753589172Z stdout")) // Missing flag
	f.Add([]byte("stdout F message without timestamp"))    // Missing timestamp
	f.Add([]byte("not-a-timestamp stdout F message"))      // Invalid timestamp

	// Messages with many spaces
	f.Add([]byte("2018-09-20T11:54:11.753589172Z stdout F   message   with   spaces   "))

	// Very long content
	longContent := strings.Repeat("A", 10000)
	f.Add([]byte(fmt.Sprintf("2018-09-20T11:54:11.753589172Z stdout F %s", longContent)))

	// Unknown stream types
	f.Add([]byte("2018-09-20T11:54:11.753589172Z unknown F message"))
	f.Add([]byte("2018-09-20T11:54:11.753589172Z STDOUT F message")) // Different case

	// Unknown flags
	f.Add([]byte("2018-09-20T11:54:11.753589172Z stdout X message"))
	f.Add([]byte("2018-09-20T11:54:11.753589172Z stdout f message")) // Lowercase

	// Content that looks like Kubernetes format
	f.Add([]byte("2018-09-20T11:54:11.753589172Z stdout F 2018-09-20T11:54:11.753589172Z stdout F nested"))

	parser := New()

	f.Fuzz(func(t *testing.T, data []byte) {
		msg := message.NewMessage(data, nil, "", 0)

		// Parser should not panic
		result, err := parser.Parse(msg)

		// Verify invariants
		if err == nil && result != nil {
			// Status should be one of the valid values
			if result.Status != "" {
				if result.Status != message.StatusInfo &&
					result.Status != message.StatusError {
					t.Errorf("Invalid status: %s", result.Status)
				}
			}

			// If IsPartial is set, flag must have been "P"
			if result.ParsingExtra.IsPartial {
				// Verify the original message had "P" flag
				components := strings.Split(string(data), " ")
				if len(components) >= 3 && components[2] != "P" {
					t.Errorf("IsPartial is true but flag was %q", components[2])
				}
			}

			// If we have a timestamp, it should be the first component
			if result.ParsingExtra.Timestamp != "" {
				if !strings.HasPrefix(string(data), result.ParsingExtra.Timestamp) {
					t.Errorf("Timestamp %q not found at start of message", result.ParsingExtra.Timestamp)
				}

				// Verify it's a valid timestamp format
				_, err := time.Parse(time.RFC3339Nano, result.ParsingExtra.Timestamp)
				if err != nil {
					// Kubernetes uses RFC3339Nano format
					t.Errorf("Timestamp %q is not valid RFC3339Nano format: %v", result.ParsingExtra.Timestamp, err)
				}
			}

		} else if err != nil {
			// If parsing failed, verify the error makes sense
			// The parser returns an error for:
			// 1. Messages with < 3 components
			// 2. Messages with invalid timestamp in first component
			components := strings.Split(string(data), " ")
			if len(components) >= 3 && err.Error() != "invalid timestamp format" {
				// If it's not a timestamp error, it should have parsed successfully
				if len(components[0]) > 0 && len(components[1]) > 0 && len(components[2]) > 0 {
					t.Errorf("Parser returned unexpected error for message with %d components: %v", len(components), err)
				}
			}
		}
	})
}

func FuzzIsPartial(f *testing.F) {
	// Test the isPartial function directly
	f.Add("P")
	f.Add("F")
	f.Add("p")
	f.Add("f")
	f.Add("")
	f.Add("PP")
	f.Add("partial")
	f.Add("full")

	f.Fuzz(func(t *testing.T, flag string) {
		result := isPartial(flag)

		// Only "P" should return true
		expected := flag == "P"
		if result != expected {
			t.Errorf("isPartial(%q) = %v, want %v", flag, result, expected)
		}
	})
}

func FuzzGetStatus(f *testing.F) {
	// Test the getStatus function directly
	f.Add([]byte("stdout"))
	f.Add([]byte("stderr"))
	f.Add([]byte("STDOUT"))
	f.Add([]byte("STDERR"))
	f.Add([]byte(""))
	f.Add([]byte("unknown"))
	f.Add([]byte("stdin"))

	f.Fuzz(func(t *testing.T, streamType []byte) {
		result := getStatus(streamType)

		// Verify the result is one of the valid statuses
		switch result {
		case message.StatusInfo, message.StatusError:
			// Valid status
		default:
			t.Errorf("getStatus(%q) returned invalid status: %q", streamType, result)
		}

		// Verify specific mappings
		switch string(streamType) {
		case "stdout":
			if result != message.StatusInfo {
				t.Errorf("getStatus(\"stdout\") = %q, want %q", result, message.StatusInfo)
			}
		case "stderr":
			if result != message.StatusError {
				t.Errorf("getStatus(\"stderr\") = %q, want %q", result, message.StatusError)
			}
		default:
			// Everything else should default to INFO
			if result != message.StatusInfo {
				t.Errorf("getStatus(%q) = %q, want %q (default)", streamType, result, message.StatusInfo)
			}
		}
	})
}
