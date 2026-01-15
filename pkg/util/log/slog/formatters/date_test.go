// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDate(t *testing.T) {
	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC)

	formatter := Date(false)
	result := formatter(testTime)

	assert.Equal(t, "2023-11-04 15:30:45 UTC", result)
}

func TestDateRFC3339(t *testing.T) {
	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC)

	formatter := Date(true)
	result := formatter(testTime)

	assert.Equal(t, "2023-11-04T15:30:45Z", result)
}

func TestDateWithTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("Timezone not available")
	}

	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, loc)

	formatter := Date(false)
	result := formatter(testTime)

	assert.Equal(t, result, "2023-11-04 15:30:45 EDT")
}

func TestDateRFC3339WithTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("Timezone not available")
	}

	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, loc)

	formatter := Date(true)
	result := formatter(testTime)

	assert.Equal(t, result, "2023-11-04T15:30:45-04:00")
}

func TestDateConsistency(t *testing.T) {
	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC)

	formatter := Date(false)

	// Multiple calls should return the same result
	result1 := formatter(testTime)
	result2 := formatter(testTime)

	assert.Equal(t, result1, result2)
}

func TestDateZeroTime(t *testing.T) {
	testTime := time.Time{}

	formatter := Date(false)
	result := formatter(testTime)

	// Should not panic with zero time
	assert.NotEmpty(t, result)
}

func TestGetLogDateFormat(t *testing.T) {
	// Test default format
	format := GetLogDateFormat(false)
	assert.Equal(t, logDateFormat, format)

	// Test RFC3339 format
	format = GetLogDateFormat(true)
	assert.Equal(t, time.RFC3339, format)
}

func TestDateWithNanoseconds(t *testing.T) {
	testTime := time.Date(2023, 11, 4, 15, 30, 45, 123456789, time.UTC)

	formatter := Date(false)
	result := formatter(testTime)

	// Default format doesn't include nanoseconds
	assert.Equal(t, "2023-11-04 15:30:45 UTC", result)
}

func TestDateRFC3339WithNanoseconds(t *testing.T) {
	testTime := time.Date(2023, 11, 4, 15, 30, 45, 123456789, time.UTC)

	formatter := Date(true)
	result := formatter(testTime)

	assert.Equal(t, result, "2023-11-04T15:30:45Z")
}
