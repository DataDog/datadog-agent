// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procinfo

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBoundedCPUInteger(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	tests := []struct {
		name string
		cpu  float64
		want int
	}{
		{name: "not a number", cpu: math.NaN(), want: 0},
		{name: "negative infinity", cpu: math.Inf(-1), want: 0},
		{name: "negative", cpu: -1.5, want: 0},
		{name: "zero", cpu: 0, want: 0},
		{name: "fractional", cpu: 12.9, want: 12},
		{name: "platform maximum", cpu: float64(maxInt), want: maxInt},
		{name: "above platform maximum", cpu: float64(maxInt) * 2, want: maxInt},
		{name: "positive infinity", cpu: math.Inf(1), want: maxInt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, boundedCPUInteger(tt.cpu))
		})
	}
}

func TestTruncateCmdNameSanitizesUnsafeCharacters(t *testing.T) {
	name := "safe\x00\nline\tindent\x1b[31m\x7f" +
		string([]byte{0xff}) +
		"\u2028worker"

	got := truncateCmdName(name)

	require.Equal(t, "safe??line?indent?[31m???worker", got)
}

func TestTruncateCmdNameSanitizesEveryASCIIControl(t *testing.T) {
	controls := make([]byte, 0, 33)
	for control := byte(0); control < ' '; control++ {
		controls = append(controls, control)
	}
	controls = append(controls, 0x7f)

	require.Equal(t, strings.Repeat("?", len(controls)), truncateCmdName(string(controls)))
}

func TestTruncateCmdNamePreservesPrintableUnicode(t *testing.T) {
	name := "safe worker 日本語 café"

	require.Equal(t, name, truncateCmdName(name))
}

func TestTruncateCmdNameEnforcesByteLimitAtRuneBoundary(t *testing.T) {
	t.Run("exact limit", func(t *testing.T) {
		name := strings.Repeat("a", MaxCmdLen)

		require.Equal(t, name, truncateCmdName(name))
	})

	t.Run("complete multibyte rune fits", func(t *testing.T) {
		name := strings.Repeat("a", MaxCmdLen-2) + "é" + "z"
		got := truncateCmdName(name)

		require.Equal(t, strings.Repeat("a", MaxCmdLen-2)+"é", got)
	})

	t.Run("multibyte rune would be split", func(t *testing.T) {
		name := strings.Repeat("a", MaxCmdLen-1) + "é"
		got := truncateCmdName(name)

		require.Equal(t, strings.Repeat("a", MaxCmdLen-1), got)
	})
}

func TestFormatStartTime(t *testing.T) {
	now := time.Date(2026, time.July, 30, 12, 0, 0, 0, time.Local)

	require.Equal(t, "09:08", formatStartTime(
		time.Date(2026, time.July, 30, 9, 8, 0, 0, time.Local),
		now,
	))
	require.Equal(t, "Jul29", formatStartTime(
		time.Date(2026, time.July, 29, 9, 8, 0, 0, time.Local),
		now,
	))
}
