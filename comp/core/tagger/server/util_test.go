// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockStreamTagsEvent struct {
	id   int
	size int
}

func TestSplitEvents(t *testing.T) {
	testCases := []struct {
		name         string
		events       []mockStreamTagsEvent
		maxChunkSize int
		expected     [][]mockStreamTagsEvent // Expecting indices of events in chunks for easier comparison
	}{
		{
			name:         "Empty input",
			events:       []mockStreamTagsEvent{},
			maxChunkSize: 100,
			expected:     nil, // No chunks expected
		},
		{
			name: "Single event within chunk size",
			events: []mockStreamTagsEvent{
				{id: 1, size: 50}, // Mock event with size 50
			},
			maxChunkSize: 100,
			expected: [][]mockStreamTagsEvent{
				{
					{id: 1, size: 50}, // One chunk with one event
				},
			},
		},
		{
			name: "Multiple events all fit in one chunk",
			events: []mockStreamTagsEvent{
				{id: 1, size: 20}, {id: 2, size: 30}, {id: 3, size: 40}, // Total size = 90
			},
			maxChunkSize: 100,
			expected: [][]mockStreamTagsEvent{
				{
					{id: 1, size: 20}, {id: 2, size: 30}, {id: 3, size: 40}, // All events fit in one chunk
				},
			},
		},
		{
			name: "Multiple events require splitting",
			events: []mockStreamTagsEvent{
				{id: 1, size: 40}, {id: 2, size: 50}, {id: 3, size: 60}, // Total size = 150
			},
			maxChunkSize: 100,
			expected: [][]mockStreamTagsEvent{
				{
					{id: 1, size: 40},
					{id: 2, size: 50},
				},
				{
					{id: 3, size: 60},
				}, // Last event in second chunk
			},
		},
		{
			name: "Events fit exactly in chunks",
			events: []mockStreamTagsEvent{
				{id: 1, size: 50}, {id: 2, size: 50}, // Total size = 100
			},
			maxChunkSize: 100,
			expected: [][]mockStreamTagsEvent{
				{{id: 1, size: 50}, {id: 2, size: 50}}, // Both events fit exactly in one chunk
			},
		},
		{
			name: "Event size exactly matches or exceeds chunk size",
			events: []mockStreamTagsEvent{
				{id: 1, size: 100}, {id: 2, size: 101}, // One exactly fits, one exceeds
			},
			maxChunkSize: 100,
			expected: [][]mockStreamTagsEvent{
				{
					{id: 1, size: 100},
				},
				{
					{id: 2, size: 101},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			chunks := splitBySize(testCase.events, testCase.maxChunkSize, func(e mockStreamTagsEvent) int { return e.size })
			assert.Equal(t, testCase.expected, chunks)
		})
	}
}
