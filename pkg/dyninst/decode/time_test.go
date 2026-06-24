// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"encoding/binary"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// timeAsBytes extracts the in-memory representation of a time.Time as the
// 24-byte buffer a probe would capture (wall, ext, loc). Field offsets match
// the layout in src/time/time.go: wall at 0, ext at 8.
func timeAsBytes(t time.Time) []byte {
	bytes := make([]byte, unsafe.Sizeof(t))
	*(*time.Time)(unsafe.Pointer(&bytes[0])) = t
	return bytes
}

func TestDecodeGoTime(t *testing.T) {
	const (
		wallOff = 0
		extOff  = 8
	)

	t.Run("zero", func(t *testing.T) {
		sec, nsec, isZero := decodeGoTime(timeAsBytes(time.Time{}), wallOff, extOff)
		require.True(t, isZero)
		require.Zero(t, sec)
		require.Zero(t, nsec)
	})

	t.Run("monotonic_now", func(t *testing.T) {
		// time.Now() sets hasMonotonic.
		want := time.Now()
		sec, nsec, isZero := decodeGoTime(timeAsBytes(want), wallOff, extOff)
		require.False(t, isZero)
		require.Equal(t, want.Unix(), sec)
		require.Equal(t, uint32(want.Nanosecond()), nsec)
	})

	t.Run("non_monotonic_unix", func(t *testing.T) {
		// time.Unix has no monotonic reading; sec lives in ext.
		want := time.Unix(1_700_000_000, 123_456_789)
		sec, nsec, isZero := decodeGoTime(timeAsBytes(want), wallOff, extOff)
		require.False(t, isZero)
		require.Equal(t, int64(1_700_000_000), sec)
		require.Equal(t, uint32(123_456_789), nsec)
	})

	t.Run("epoch", func(t *testing.T) {
		want := time.Unix(0, 0)
		sec, nsec, isZero := decodeGoTime(timeAsBytes(want), wallOff, extOff)
		require.False(t, isZero)
		require.Equal(t, int64(0), sec)
		require.Equal(t, uint32(0), nsec)
	})

	t.Run("pre_unix_epoch", func(t *testing.T) {
		// Year 1900 — well before Unix epoch, no monotonic.
		want := time.Date(1900, 6, 15, 12, 0, 0, 0, time.UTC)
		sec, nsec, isZero := decodeGoTime(timeAsBytes(want), wallOff, extOff)
		require.False(t, isZero)
		require.Equal(t, want.Unix(), sec)
		require.Equal(t, uint32(0), nsec)
	})

	t.Run("future_pre_2157", func(t *testing.T) {
		// Far future but inside the 33-bit monotonic range.
		want := time.Date(2100, 1, 2, 3, 4, 5, 6, time.UTC)
		sec, nsec, isZero := decodeGoTime(timeAsBytes(want), wallOff, extOff)
		require.False(t, isZero)
		require.Equal(t, want.Unix(), sec)
		require.Equal(t, uint32(6), nsec)
	})

	t.Run("non_utc_location_decoded_as_utc_instant", func(t *testing.T) {
		// Decoder ignores loc — the wall instant must still match.
		want := time.Now().In(time.FixedZone("X", 3600))
		sec, nsec, isZero := decodeGoTime(timeAsBytes(want), wallOff, extOff)
		require.False(t, isZero)
		require.Equal(t, want.Unix(), sec)
		require.Equal(t, uint32(want.Nanosecond()), nsec)
	})

	t.Run("short_buffer_treated_as_zero", func(t *testing.T) {
		_, _, isZero := decodeGoTime(make([]byte, 8), wallOff, extOff)
		require.True(t, isZero)
	})
}

// makeTimeBuf builds a synthetic 24-byte time.Time capture buffer the way
// BPF would after running SM_OP_PROCESS_GO_TIME: wall and ext as Go's
// runtime would lay them out, and the high 8 bytes set to the resolved
// offset (or the unresolved sentinel).
func makeTimeBuf(t time.Time, offsetSlot int64) []byte {
	buf := timeAsBytes(t)
	binary.NativeEndian.PutUint64(buf[16:24], uint64(offsetSlot))
	return buf
}

func TestGoTimeTypeFormat(t *testing.T) {
	timeType := &goTimeType{GoTimeType: &ir.GoTimeType{
		WallFieldOffset: 0,
		ExtFieldOffset:  8,
		LocFieldOffset:  16,
	}}

	t.Run("utc_sentinel_renders_Z", func(t *testing.T) {
		buf := makeTimeBuf(
			time.Unix(1700000000, 123456789),
			ir.GoTimeUnresolvedOffset,
		)
		got, isZero := timeType.format(buf)
		require.False(t, isZero)
		require.Equal(t, "2023-11-14T22:13:20.123456789Z", got)
	})

	t.Run("positive_offset_renders_with_sign", func(t *testing.T) {
		buf := makeTimeBuf(
			time.Unix(1700000000, 0),
			3600, // +01:00
		)
		got, isZero := timeType.format(buf)
		require.False(t, isZero)
		require.Equal(t, "2023-11-14T23:13:20+01:00", got)
	})

	t.Run("negative_offset_renders_with_sign", func(t *testing.T) {
		buf := makeTimeBuf(
			time.Unix(1700000000, 0),
			-5*3600, // -05:00 (EST)
		)
		got, isZero := timeType.format(buf)
		require.False(t, isZero)
		require.Equal(t, "2023-11-14T17:13:20-05:00", got)
	})

	t.Run("zero_time_returns_isZero", func(t *testing.T) {
		buf := makeTimeBuf(time.Time{}, ir.GoTimeUnresolvedOffset)
		_, isZero := timeType.format(buf)
		require.True(t, isZero)
	})

	t.Run("short_buffer_falls_back_to_utc", func(t *testing.T) {
		// 16-byte buffer (no loc slot) — decoder must not panic and
		// must default to UTC rendering.
		buf := timeAsBytes(time.Unix(1700000000, 0))[:16]
		got, isZero := timeType.format(buf)
		require.False(t, isZero)
		require.Equal(t, "2023-11-14T22:13:20Z", got)
	})
}

func TestDecodeGoTimeRFC3339Format(t *testing.T) {
	// End-to-end sanity: decode then format via the same code path the
	// production decoder uses. We don't go through goTimeType (which needs
	// an ir.StructureType); we just confirm the formatting choice.
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"unix_zero", time.Unix(0, 0), "1970-01-01T00:00:00Z"},
		{"with_nanos", time.Unix(1700000000, 123456789), "2023-11-14T22:13:20.123456789Z"},
		{"non_utc_input", time.Unix(1700000000, 0).In(time.FixedZone("X", 3600)), "2023-11-14T22:13:20Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sec, nsec, isZero := decodeGoTime(timeAsBytes(tc.in), 0, 8)
			require.False(t, isZero)
			got := time.Unix(sec, int64(nsec)).UTC().Format(time.RFC3339Nano)
			require.Equal(t, tc.want, got)
		})
	}
}
