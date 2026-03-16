// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

import (
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateIntakeTimeOffset_EmptyHeader(t *testing.T) {
	// Set a known value first
	intakeTimeOffsetExpvar.Set(123.45)

	// Update with empty header
	updateIntakeTimeOffset("")

	// Value should remain unchanged
	offset := intakeTimeOffsetExpvar.Value()
	assert.Equal(t, 123.45, offset, "offset should remain unchanged with empty header")
}

func TestUpdateIntakeTimeOffset_InvalidDateFormat(t *testing.T) {
	// Set a known value first
	intakeTimeOffsetExpvar.Set(123.45)

	// Update with invalid date format
	updateIntakeTimeOffset("not-a-valid-date")

	// Value should remain unchanged
	offset := intakeTimeOffsetExpvar.Value()
	assert.Equal(t, 123.45, offset, "offset should remain unchanged with invalid date format")
}

func TestUpdateIntakeTimeOffset_SignConvention(t *testing.T) {
	baseTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	mockClock := clock.NewMock()
	mockClock.Set(baseTime)

	testCases := []struct {
		name           string
		serverTimeDiff time.Duration
		expectedOffset float64
	}{
		{"agent 1 minute ahead", -1 * time.Minute, -60.0},
		{"agent 1 minute behind", 1 * time.Minute, 60.0},
		{"agent synchronized", 0, 0.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			intakeTimeOffsetExpvar.Set(0)
			serverTime := baseTime.Add(tc.serverTimeDiff)
			dateHeader := serverTime.Format(http.TimeFormat)
			updateIntakeTimeOffsetWithClock(dateHeader, mockClock)

			offset := intakeTimeOffsetExpvar.Value()
			require.False(t, math.IsNaN(offset), "offset should not be NaN")
			assert.InDelta(t, tc.expectedOffset, offset, 0.01,
				"offset should be %.1f seconds", tc.expectedOffset)
		})
	}
}
