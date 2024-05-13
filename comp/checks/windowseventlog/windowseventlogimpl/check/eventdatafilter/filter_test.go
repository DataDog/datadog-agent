// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

package eventdatafilter

import (
	"fmt"

	"github.com/stretchr/testify/assert"
	"testing"
)

func TestIsAllowedEventID(t *testing.T) {

	f := &eventIDFilter{eventIDs: []int{1000, 2000}}
	tcs := []struct {
		eventID int
		match   bool
	}{
		{1000, true},
		{2000, true},
		{3000, false},
	}
	for _, tc := range tcs {
		match := f.isAllowedEventID(tc.eventID)
		assert.Equal(t, tc.match, match)
	}

	f = &eventIDFilter{eventIDs: []int{}}
	match := f.isAllowedEventID(1000)
	assert.True(t, match)
}

func BenchmarkIsAllowedEventID(b *testing.B) {
	sizeMatchSet := []int{10, 100, 1000, 10000}
	for _, n := range sizeMatchSet {
		b.Run(fmt.Sprintf("MatchEventID_%d", n), func(b *testing.B) {
			// build a filter with n elements
			f := &eventIDFilter{eventIDs: make([]int, n)}
			for i := 0; i < n; i++ {
				f.eventIDs[i] = i
			}
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				// set eventID to the size to imitate worst case scenario
				f.isAllowedEventID(n)
			}
		})
	}
}
