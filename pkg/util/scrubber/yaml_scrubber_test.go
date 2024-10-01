// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrubDataObj(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "Scrub sensitive info from map",
			input: map[string]interface{}{
				"password": "password123",
				"username": "user1",
			},
			expected: map[string]interface{}{
				"password": "********",
				"username": "user1",
			},
		},
		{
			name: "Scrub sensitive info from nested map",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"password": "password123",
					"email":    "user@example.com",
				},
			},
			expected: map[string]interface{}{
				"user": map[string]interface{}{
					"password": "********",
					"email":    "user@example.com",
				},
			},
		},
		{
			name:     "No sensitive info to scrub",
			input:    "Just a regular string.",
			expected: "Just a regular string.",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ScrubDataObj(&tc.input)
			assert.Equal(t, tc.expected, tc.input)
		})
	}
}
