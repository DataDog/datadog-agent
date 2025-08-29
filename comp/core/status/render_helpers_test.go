// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNtpWarning(t *testing.T) {
	require.False(t, ntpWarning(1))
	require.False(t, ntpWarning(-1))
	require.True(t, ntpWarning(3601))
	require.True(t, ntpWarning(-601))
}

func TestMkHuman(t *testing.T) {
	f := 1695783.0
	fStr := mkHuman(f)
	assert.Equal(t, "1,695,783", fStr, "Large number formatting is incorrectly adding commas in agent statuses")

	assert.Equal(t, "1", mkHuman(1))
	assert.Equal(t, "1", mkHuman("1"))
	assert.Equal(t, "1.5", mkHuman(float32(1.5)))
}

func TestParseUnixTime(t *testing.T) {
	cases := []struct {
		value          any
		expectedOutput time.Time
	}{
		{int64(1756201396), time.Unix(1756201396, 0)},
		{int64(1756199835000000000), time.Unix(0, 1756199835000000000)}, // nanoseconds
		{float64(1756201396.123), time.Unix(1756201396, 0)},
		{"2025-08-26T11:43:16.000000+02:00", time.Unix(1756201396, 0)},
	}

	for _, tc := range cases {
		output, err := parseUnixTime(tc.value)
		if assert.NoError(t, err) {
			assert.WithinDuration(t, tc.expectedOutput, output, 0)
		}
	}
}

func TestParseUnixTimeError(t *testing.T) {
	cases := []struct {
		value            any
		expectedErrorMsg string
	}{
		{false, "invalid time parameter bool"},
		{"Tue Aug 26 11:43:16 CEST 2025", "error while parsing time: Tue Aug 26 11:43:16 CEST 2025"},
	}

	for _, tc := range cases {
		output, err := parseUnixTime(tc.value)
		assert.EqualError(t, err, tc.expectedErrorMsg)
		assert.Zero(t, output)
	}
}
