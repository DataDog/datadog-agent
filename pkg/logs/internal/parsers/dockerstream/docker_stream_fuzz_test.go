// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dockerstream

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func FuzzParseDockerStream(f *testing.F) {
	// Helper to create valid Docker header
	createHeader := func(streamType byte, size uint32) []byte {
		header := make([]byte, 8)
		header[0] = streamType
		// Size in big-endian format
		header[4] = byte(size >> 24)
		header[5] = byte(size >> 16)
		header[6] = byte(size >> 8)
		header[7] = byte(size)
		return header
	}

	// Helper to create RFC3339 timestamp variations
	timestamps := []string{
		"2018-06-14T18:27:03.246999277Z",
		"2018-06-14T18:27:03Z",
		"2018-06-14T18:27:03.123Z",
		"2018-06-14T18:27:03.123456789Z",
		"2018-06-14T18:27:03+00:00",
		"2018-06-14T18:27:03.123+00:00",
		"2018-06-14T18:27:03-07:00",
		"2018-06-14T18:27:03.123456-07:00",
	}

	// Valid messages with headers
	for _, ts := range timestamps {
		// stdout messages
		msg := fmt.Sprintf("%s valid log message", ts)
		f.Add(append(createHeader(1, uint32(len(msg))), []byte(msg)...))

		// stderr messages
		msg = fmt.Sprintf("%s error log message", ts)
		f.Add(append(createHeader(2, uint32(len(msg))), []byte(msg)...))

		// Empty content after timestamp
		msg = fmt.Sprintf("%s ", ts)
		f.Add(append(createHeader(1, uint32(len(msg))), []byte(msg)...))
	}

	// TTY messages (no header) with valid timestamps
	for _, ts := range timestamps {
		f.Add([]byte(fmt.Sprintf("%s tty message without header", ts)))
		f.Add([]byte(ts))                     // Just timestamp
		f.Add([]byte(fmt.Sprintf("%s ", ts))) // Timestamp with space
	}

	// Large messages that trigger partial handling
	largeContent := make([]byte, dockerBufferSize+100)
	for i := range largeContent {
		largeContent[i] = 'A' + byte(i%26)
	}
	ts := "2018-06-14T18:27:03.246999277Z"
	largeMsg := fmt.Sprintf("%s %s", ts, string(largeContent))
	f.Add(append(createHeader(1, uint32(len(largeMsg))), []byte(largeMsg)...))

	// Edge cases with valid format
	f.Add([]byte{})            // Empty input (could happen with empty log read)
	f.Add(createHeader(1, 0))  // Header with zero size (empty log line)
	f.Add(createHeader(1, 10)) // Header claiming size but truncated content (network issue/buffer cutoff)

	// Messages with escape sequences
	for _, escape := range []string{"\\n", "\\r", "\\r\\n"} {
		msg := fmt.Sprintf("%s %s", ts, escape)
		f.Add([]byte(msg))                                               // TTY format
		f.Add(append(createHeader(1, uint32(len(msg))), []byte(msg)...)) // With header
	}

	parser := New("fuzz_container")

	f.Fuzz(func(t *testing.T, data []byte) {
		msg := message.NewMessage(data, nil, "", 0)

		// The parser should not panic on any input
		result, err := parser.Parse(msg)

		// Verify invariants
		if err == nil && result != nil {
			// If we have a valid status, it should be Info or Error
			if result.Status != "" {
				if result.Status != message.StatusInfo &&
					result.Status != message.StatusError {
					t.Errorf("Invalid status: %s", result.Status)
				}
			}
		}
	})
}

func FuzzRemovePartialDockerMetadata(f *testing.F) {
	// Helper to create a partial message with valid header and timestamp
	createPartialMessage := func(content []byte, streamType byte) []byte {
		header := make([]byte, 8)
		header[0] = streamType
		timestamp := []byte("2018-06-14T18:27:03.246999277Z ")
		return append(append(header, timestamp...), content...)
	}

	// Seed corpus with realistic scenarios

	// Single chunk under buffer size
	smallContent := make([]byte, 100)
	for i := range smallContent {
		smallContent[i] = 'a'
	}
	f.Add(createPartialMessage(smallContent, 1))

	// Exactly at buffer size
	exactContent := make([]byte, dockerBufferSize)
	for i := range exactContent {
		exactContent[i] = 'b'
	}
	f.Add(createPartialMessage(exactContent, 1))

	// Multiple chunks
	largeContent := make([]byte, dockerBufferSize*2+500)
	for i := range largeContent {
		largeContent[i] = 'c' + byte(i%26)
	}
	f.Add(createPartialMessage(largeContent, 2))

	// Edge cases
	f.Add([]byte{})
	f.Add([]byte("no header just text"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Function should not panic
		result := removePartialDockerMetadata(data)

		// Result should never be longer than input
		if len(result) > len(data) {
			t.Errorf("removePartialDockerMetadata increased size from %d to %d", len(data), len(result))
		}

		// If input was empty, output should be empty
		if len(data) == 0 && len(result) != 0 {
			t.Errorf("removePartialDockerMetadata returned non-empty result for empty input")
		}

	})
}
