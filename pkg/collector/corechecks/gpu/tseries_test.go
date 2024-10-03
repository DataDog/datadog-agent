// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTseriesBuilder(t *testing.T) {
	events := []struct {
		start uint64
		end   uint64
		value int64
	}{
		// Test overlapping events
		{0, 10, 1},
		{5, 15, 2},

		// Test events with gaps
		{20, 30, 10},
		// Test events with same end
		{28, 30, 3},
		// Test events with same start
		{31, 33, 4},
		{31, 34, 5},

		// Test gap plus events with no end
		{40, 50, 8},
	}

	onlystarts := []struct {
		start uint64
		value int64
	}{
		{35, 7},
	}

	builder := tseriesBuilder{}
	for _, e := range events {
		builder.AddEvent(e.start, e.end, e.value)
	}
	for _, s := range onlystarts {
		builder.AddEventStart(s.start, s.value)
	}

	tseries, max := builder.Build()
	assert.Equal(t, max, int64(15)) // From event [40,50]=8 and onlystarts [35,inf]=7a

	expected := []tsPoint{
		{0, 1},
		{5, 3},
		{10, 2},
		{15, 0},
		{20, 10},
		{28, 13},
		{30, 0},
		{31, 9},
		{33, 5},
		{34, 0},
		{35, 7},
		{40, 15},
		{50, 7},
	}

	require.ElementsMatch(t, expected, tseries)
}
