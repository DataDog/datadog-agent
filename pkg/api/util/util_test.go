// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsIPv6(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOutput bool
	}{
		{
			name:           "IPv4",
			input:          "192.168.0.1",
			expectedOutput: false,
		},
		{
			name:           "IPv6",
			input:          "2600:1f19:35d4:b900:527a:764f:e391:d369",
			expectedOutput: true,
		},
		{
			name:           "zero compressed IPv6",
			input:          "2600:1f19:35d4:b900::1",
			expectedOutput: true,
		},
		{
			name:           "IPv6 loopback",
			input:          "::1",
			expectedOutput: true,
		},
		{
			name:           "short hostname with only hexadecimal digits",
			input:          "cafe",
			expectedOutput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, IsIPv6(tt.input), tt.expectedOutput)
		})
	}
}

// TestValidateConstantTimeComparison is a test function that validates the constant time comparison
// between two strings of varying lengths. It generates random strings and compares them using both
// a constant time comparison function and a trivial comparison function. The durations of the two
// comparison functions are logged for each string length.
func TestValidateConstantTimeComparison(t *testing.T) {
	maxLength := 100
	repetitionAmount := 100
	for length := 1; length < maxLength; length += maxLength / 50 {
		base := generateRandomString(length)

		var stringsSlice []string
		for i := 0; i < length; i++ {
			// prefixLength := rand.Intn(i) // Random prefix length
			prefix := base[:i]

			// Generate a suffix with random characters to fill the rest of the string
			suffixLength := length - i
			suffix := generateRandomString(suffixLength)

			stringsSlice = append(stringsSlice, prefix+suffix)
		}

		var constFuncDurations []time.Duration
		var trivialFuncDurations []time.Duration

		var startTime time.Time // init time before loop
		var constantRes, trivialRes bool
		for _, intent := range stringsSlice {

			// Capture the start time
			startTime = time.Now()
			for range repetitionAmount {
				constantRes = constantTimeCompareTokens(intent, base)
			}
			constFuncDurations = append(constFuncDurations, time.Since(startTime))

			startTime = time.Now()
			for range repetitionAmount {
				trivialRes = trivialCompare(intent, base)
			}
			trivialFuncDurations = append(trivialFuncDurations, time.Since(startTime))

			assert.Equal(t, constantRes, trivialRes)

		}

		t.Logf("for len %d: %s vs %s", length, durationDifference(constFuncDurations), durationDifference(trivialFuncDurations))
	}

}

func trivialCompare(a, b string) bool {
	return a == b
}

// generateRandomString generates a random string of a given length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func durationDifference(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	shortest := time.Duration(math.MaxInt64)
	longest := time.Duration(0)

	for _, duration := range durations {
		if duration < shortest {
			shortest = duration
		}
		if duration > longest {
			longest = duration
		}
	}

	return longest - shortest
}
