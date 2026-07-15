// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import "time"

// testTimeUTC and testTimeFixedZone use literal Unix timestamps so the
// captured instant is deterministic across runs; their snapshot values
// are pinned in testdata.
//
// testTimeMonotonic captures time.Now() which carries a monotonic clock
// reading. Its value is non-deterministic, so the test harness's redactor
// asserts the captured timestamp falls within a known window and replaces
// it with a sentinel.

// timeCapture exists to give the monotonic time.Time a recognizable
// field name (monotonicNow) so the redactor matcher can scope to it.
type timeCapture struct {
	monotonicNow time.Time
}

//nolint:all
//go:noinline
func testTimeUTC(x time.Time) {}

//nolint:all
//go:noinline
func testTimeFixedZone(x time.Time) {}

//nolint:all
//go:noinline
func testTimeMonotonic(c timeCapture) {}

//nolint:all
//go:noinline
func executeTimeFuncs() {
	// 2023-11-14T22:13:20Z (Unix 1_700_000_000) — past, no monotonic.
	t := time.Unix(1_700_000_000, 123_456_789).UTC()
	testTimeUTC(t)

	// Same instant, observed through a +05:30 (IST) fixed zone. The
	// captured loc pointer points at a process-local *Location whose
	// cache must be warmed for the BPF fast path to find an offset; we
	// warm it by calling Format() once.
	ist := time.FixedZone("IST", 5*3600+30*60)
	tIST := time.Unix(1_700_000_000, 123_456_789).In(ist)
	_ = tIST.Format(time.RFC3339Nano)
	testTimeFixedZone(tIST)

	// time.Now() sets hasMonotonic; exercising it confirms the decoder
	// extracts wall seconds from the packed 33-bit field rather than
	// reading ext (which holds the monotonic delta in this case).
	testTimeMonotonic(timeCapture{monotonicNow: time.Now()})
}
