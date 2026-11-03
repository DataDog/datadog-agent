// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package free

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/meminfo"
)

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		v    uint64
		want string
	}{
		{0, "0B"},
		{1, "1B"},
		{1023, "1023B"},
		{1024, "1.0Ki"},
		{1536, "1.5Ki"},
		{10 * 1024, "10Ki"}, // ≥10 drops the decimal
		{1024 * 1024, "1.0Mi"},
		{1 << 30, "1.0Gi"},
		{1 << 40, "1.0Ti"},
		{1 << 50, "1.0Pi"},
		{1 << 60, "1.0Ei"},
		// procps-ng free rounds to nearest (not always up, unlike df's
		// human-readable mode): 1535 bytes is 1.4990...Ki, which rounds
		// to 1.5Ki, and 1536 is exactly 1.5Ki.
		{1535, "1.5Ki"},
		{1583, "1.5Ki"}, // 1.546Ki rounds down to 1.5Ki
		{1587, "1.5Ki"}, // still rounds to 1.5 at one decimal place
		{1638, "1.6Ki"}, // 1.6001...Ki rounds to 1.6Ki
		// Promotion at the boundary: rounding must not emit "1024.0Ki".
		{1024*1024 - 1, "1.0Mi"},
		// Never panics or exceeds the suffix table at the uint64 max.
		{^uint64(0), "16Ei"},
		// Exact decimal ties round half-to-even, matching Go's/C's
		// correctly-rounded %.1f formatting (and hence procps-ng's own
		// plain printf) rather than a naive round-half-up: 1280 bytes is
		// exactly 1.25Ki, and 1310720 bytes is exactly 1.25Mi. Both ties
		// round to the even neighbor (1.2), not 1.3.
		{1280, "1.2Ki"},
		{1280 * 1024, "1.2Mi"},
		// 1.75 is also an exact tie; the even neighbor here is 1.8.
		{1792, "1.8Ki"},
		// Field-width-driven promotion boundary (procps-ng scale_size()):
		// promotion happens when the formatted value no longer fits a
		// 5-character field, not at the true 1024 boundary of the
		// current unit. 1010Ki as an integer is "1010Ki" (6 chars, one
		// over budget), so free promotes to the next unit and prints
		// "1.0Mi" instead — even though 1010 < 1024.
		{1010 * 1024, "1.0Mi"},
		// 999Ki is exactly 5 characters ("999Ki"), so it still fits and
		// must not be promoted prematurely.
		{999 * 1024, "999Ki"},
		// 1000Ki would be "1000Ki" (6 characters, a 4-digit integer part),
		// one over the field-width budget, so it promotes too.
		{1000 * 1024, "1.0Mi"},
		// 1023Ki is one unit short of the true 1024 boundary but "1023Ki"
		// is 6 characters, so it still promotes to Mi under the
		// field-width rule.
		{1023 * 1024, "1.0Mi"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, humanBytes(c.v), "humanBytes(%d)", c.v)
	}
}

func TestSaturatingHelpers(t *testing.T) {
	assert.Equal(t, ^uint64(0), saturatingAdd(^uint64(0), 1))
	assert.Equal(t, uint64(3), saturatingAdd(1, 2))
	assert.Equal(t, uint64(0), saturatingSub(1, 2))
	assert.Equal(t, uint64(1), saturatingSub(3, 2))
}

func captureWriteOutput(info meminfo.Info, human bool) string {
	var buf bytes.Buffer
	callCtx := &builtins.CallContext{Stdout: &buf}
	writeOutput(callCtx, info, human)
	return buf.String()
}

func TestWriteOutput_HappyPath(t *testing.T) {
	info := meminfo.Info{
		MemTotal:     16 * 1024 * 1024 * 1024,
		MemFree:      8 * 1024 * 1024 * 1024,
		MemAvailable: 12 * 1024 * 1024 * 1024,
		Buffers:      1 * 1024 * 1024 * 1024,
		Cached:       2 * 1024 * 1024 * 1024,
		SReclaimable: 512 * 1024 * 1024,
		Shared:       256 * 1024 * 1024,
		SwapTotal:    2 * 1024 * 1024 * 1024,
		SwapFree:     2 * 1024 * 1024 * 1024,
	}
	out := captureWriteOutput(info, false)

	// used = MemTotal - MemAvailable = 16Gi - 12Gi = 4Gi = 4194304 KiB.
	assert.Contains(t, out, "total")
	assert.Contains(t, out, "buff/cache")
	assert.Contains(t, out, "available")
	assert.Contains(t, out, "Mem:")
	assert.Contains(t, out, "Swap:")
	assert.Contains(t, out, "4194304") // used, KiB
	assert.Contains(t, out, "0")       // swap used = SwapTotal-SwapFree = 0
}

func TestWriteOutput_AvailableExceedsTotal(t *testing.T) {
	// MemAvailable exceeding MemTotal (a corrupted or unusual
	// /proc/meminfo report) must saturate used to 0 rather than
	// underflowing/wrapping around.
	info := meminfo.Info{
		MemTotal:     1024,
		MemAvailable: 2000,
	}
	out := captureWriteOutput(info, false)
	assert.NotContains(t, out, "18446744073709551615")
}

func TestWriteOutput_HumanMode(t *testing.T) {
	info := meminfo.Info{
		MemTotal:  2 * 1024 * 1024 * 1024,
		MemFree:   1 * 1024 * 1024 * 1024,
		SwapTotal: 0,
		SwapFree:  0,
	}
	out := captureWriteOutput(info, true)
	assert.Contains(t, out, "Gi")
	assert.Contains(t, out, "0B") // swap columns are exactly zero
}
