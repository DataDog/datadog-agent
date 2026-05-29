// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis bug *demonstration* (not a fix), gated behind `antithesis_demo`. Run:
//
//	go test -tags "antithesis_demo test" \
//	    -run TestAntithesisContainerIdentifierNoCollision \
//	    ./comp/logs/auditor/impl/ -v -count=1 \
//	    2>&1 | grep -v "^[0-9]\{16\} \[Info\]"
//
// Property under test: `container-identifier-no-collision`
//
// # Claimed bug
//
// During container rotation, old and new tailers can share the same registry
// identifier ("file:"+path). The auditor's timestamp guard at updateRegistry()
// (auditor.go:386-389) is:
//
//	if v.IngestionTimestamp > ingestionTimestamp { return }
//
// The guard uses strict greater-than. If two updates for the same identifier
// arrive with EQUAL IngestionTimestamp values, the guard does NOT fire:
// "stored > incoming" is false, so the registry is always overwritten — the
// last writer wins unconditionally.
//
// Under container rotation:
//  1. The new tailer commits a high offset with timestamp T.
//  2. The old tailer's drain completes and sends a low offset, also with
//     timestamp T (same nanosecond — possible under CPU throttle, or
//     simply because the audit channel serialises updates into the same
//     wall-clock nanosecond).
//  3. Because "T > T" is false, the guard does not skip the old tailer's
//     low offset — it overwrites the registry with the stale position.
//  4. On restart the agent seeks to the stale (low) offset → duplicate storm.
//
// Even without equal timestamps, the guard design allows a strictly-higher
// timestamp to overwrite a higher offset, because the guard only compares
// timestamps — not offsets. A racing old tailer whose last message arrives
// with a timestamp higher than the new tailer's last update also overwrites
// the correct offset.
//
// # Sub-tests
//
// Sub-test 1 (EqualTimestampOldTailerWins):
//
//	Sequence: new-tailer(high-offset, ts=T) then old-tailer(low-offset, ts=T).
//	The guard fires only when stored.ts > incoming.ts. T > T is false.
//	Result: registry contains the LOW (stale) offset → regression confirmed.
//	EXPECTED TO FAIL (bug demonstrated).
//
// Sub-test 2 (HigherTimestampOldTailerWins):
//
//	Sequence: new-tailer(high-offset, ts=T) then old-tailer(low-offset, ts=T+1).
//	The guard fires only when stored.ts > incoming.ts. T > T+1 is false.
//	Result: registry contains the LOW (stale) offset → regression confirmed.
//	EXPECTED TO FAIL (bug demonstrated).
//	This sub-test proves that a racing old tailer with a slightly newer timestamp
//	also defeats the guard — the guard compares timestamps, not offsets.
//
// Sub-test 3 (LowerTimestampOldTailerBlocked):
//
//	Sequence: new-tailer(high-offset, ts=T) then old-tailer(low-offset, ts=T-1).
//	The guard fires: stored.ts (T) > incoming.ts (T-1) → return.
//	Result: registry retains the HIGH offset → guard protects correctly.
//	EXPECTED TO PASS (guard works for strictly older timestamps).
//
// Sub-test 4 (NewTailerRoundTripCorrect):
//
//	Sequence: monotonically increasing offsets with monotonically increasing
//	timestamps — normal operation, no collision. All updates accepted.
//	EXPECTED TO PASS.
//
// # Key code locations
//
//	comp/logs/auditor/impl/auditor.go:374-398  — updateRegistry()
//	comp/logs/auditor/impl/auditor.go:386-389  — timestamp guard (strict >)
//	pkg/logs/tailers/file/tailer.go:258-267    — FIXME: shared identifier
//
// # Verdict: REPRODUCED (sub-tests 1 and 2 fail; offset regresses).

package auditorimpl

import (
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// updateRegistryDirect is a thin wrapper that calls updateRegistry() directly,
// bypassing the run-loop channel. The auditor run loop is not started in these
// tests — updateRegistry is called synchronously, matching the serialised
// processing that happens inside run() (auditor.go:291-296).
func updateRegistryDirect(a *registryAuditor, identifier, offset string, ts int64) {
	a.updateRegistry(identifier, offset, "end", ts, types.Fingerprint{})
}

// committedOffset reads back the offset stored for identifier.
func committedOffset(a *registryAuditor, identifier string) string {
	return a.GetOffset(identifier)
}

// TestAntithesisContainerIdentifierNoCollision drives the auditor's updateRegistry
// function with the exact sequences that can occur during container rotation when
// old and new tailers share the same registry identifier.
func TestAntithesisContainerIdentifierNoCollision(t *testing.T) {
	const sharedID = "file:/var/log/containers/app_default_container.log"

	// Sub-test 1: old tailer arrives LAST with the SAME timestamp as new tailer.
	//
	// New tailer records offset 5000 at timestamp 1000.
	// Old tailer drain completes and records offset 100 at timestamp 1000.
	// Guard: stored.ts (1000) > incoming.ts (1000) → FALSE → update executes.
	// Registry ends up with offset 100 (regression).
	//
	// This is the equal-timestamp race: both tailers' messages are decoded
	// within the same nanosecond (common under CPU throttle when the auditor
	// channel serialises work into tight bursts).
	t.Run("EqualTimestampOldTailerWins", func(t *testing.T) {
		a := buildAuditorForDemo(t)
		a.registry = make(map[string]*RegistryEntry)

		const highOffset = "5000"
		const lowOffset = "100"
		const ts = int64(1000)

		// Step 1: new tailer reports a high offset (correct progress).
		updateRegistryDirect(a, sharedID, highOffset, ts)
		after1 := committedOffset(a, sharedID)
		t.Logf("after new-tailer update (offset=%s, ts=%d): committed=%s", highOffset, ts, after1)

		// Step 2: old tailer's drain finishes — same identifier, same ts, low offset.
		updateRegistryDirect(a, sharedID, lowOffset, ts)
		after2 := committedOffset(a, sharedID)
		t.Logf("after old-tailer update (offset=%s, ts=%d): committed=%s", lowOffset, ts, after2)

		// Monotonicity invariant: the committed offset must NEVER decrease.
		// Parse as integers for comparison (file offsets are byte positions).
		high, _ := strconv.ParseInt(highOffset, 10, 64)
		got, _ := strconv.ParseInt(after2, 10, 64)

		if got < high {
			t.Fatalf(
				"BUG DEMONSTRATED (container-identifier-no-collision / equal-timestamp):\n"+
					"  Sequence: new-tailer(offset=%s, ts=%d) → old-tailer(offset=%s, ts=%d)\n"+
					"  Guard condition: stored.IngestionTimestamp (%d) > incoming (%d) → %v\n"+
					"  Guard did NOT fire; old tailer's stale offset overwrote new tailer's.\n"+
					"  Committed offset REGRESSED: %d → %d (lost %d bytes of progress).\n"+
					"  On restart, agent seeks to offset %d and re-reads %d bytes → duplicate storm.\n"+
					"  Root cause: auditor.go:387 uses strict '>' — equal timestamps are not blocked.\n"+
					"  Fix: use '>=' in the guard, OR compare offsets directly (monotonic offset guard).",
				highOffset, ts, lowOffset, ts,
				ts, ts, false,
				high, got, high-got,
				got, high-got,
			)
		}

		t.Logf("UNEXPECTED PASS: committed offset did not regress (got=%d, high=%d)", got, high)
	})

	// Sub-test 2: old tailer arrives LAST with a HIGHER timestamp than the new tailer.
	//
	// New tailer records offset 5000 at timestamp 1000.
	// Old tailer drain completes and records offset 100 at timestamp 1001.
	// Guard: stored.ts (1000) > incoming.ts (1001) → FALSE → update executes.
	// Registry ends up with offset 100 (regression).
	//
	// This proves that the timestamp guard compares timestamps, NOT offsets.
	// A racing old tailer whose last ack arrives a nanosecond later always wins.
	t.Run("HigherTimestampOldTailerWins", func(t *testing.T) {
		a := buildAuditorForDemo(t)
		a.registry = make(map[string]*RegistryEntry)

		const highOffset = "5000"
		const lowOffset = "100"
		const tsNew = int64(1000)
		const tsOld = int64(1001) // old tailer's ack is slightly newer

		// Step 1: new tailer reports high offset.
		updateRegistryDirect(a, sharedID, highOffset, tsNew)
		after1 := committedOffset(a, sharedID)
		t.Logf("after new-tailer update (offset=%s, ts=%d): committed=%s", highOffset, tsNew, after1)

		// Step 2: old tailer drain arrives with slightly higher timestamp.
		updateRegistryDirect(a, sharedID, lowOffset, tsOld)
		after2 := committedOffset(a, sharedID)
		t.Logf("after old-tailer update (offset=%s, ts=%d): committed=%s", lowOffset, tsOld, after2)

		high, _ := strconv.ParseInt(highOffset, 10, 64)
		got, _ := strconv.ParseInt(after2, 10, 64)

		if got < high {
			t.Fatalf(
				"BUG DEMONSTRATED (container-identifier-no-collision / higher-timestamp):\n"+
					"  Sequence: new-tailer(offset=%s, ts=%d) → old-tailer(offset=%s, ts=%d)\n"+
					"  Guard condition: stored.IngestionTimestamp (%d) > incoming (%d) → %v\n"+
					"  Guard did NOT fire; old tailer's ack (ts=%d) overrides new tailer's (ts=%d).\n"+
					"  Committed offset REGRESSED: %d → %d (lost %d bytes of progress).\n"+
					"  Root cause: auditor.go:387 guard is timestamp-only — it protects against\n"+
					"  strictly-older messages but not against same-or-newer-ts lower-offset messages.\n"+
					"  A monotonic OFFSET guard (newOffset >= storedOffset → skip) would prevent this.",
				highOffset, tsNew, lowOffset, tsOld,
				tsNew, tsOld, false,
				tsOld, tsNew,
				high, got, high-got,
			)
		}

		t.Logf("UNEXPECTED PASS: committed offset did not regress (got=%d, high=%d)", got, high)
	})

	// Sub-test 3: old tailer arrives with a LOWER timestamp — guard should fire.
	//
	// New tailer records offset 5000 at timestamp 1000.
	// Old tailer drain arrives with timestamp 999 (strictly older).
	// Guard: stored.ts (1000) > incoming.ts (999) → TRUE → return (skip).
	// Registry retains offset 5000. Guard works correctly here.
	//
	// EXPECTED TO PASS. This is the only scenario the current guard protects against.
	t.Run("LowerTimestampOldTailerBlocked", func(t *testing.T) {
		a := buildAuditorForDemo(t)
		a.registry = make(map[string]*RegistryEntry)

		const highOffset = "5000"
		const lowOffset = "100"
		const tsNew = int64(1000)
		const tsOld = int64(999) // strictly older → guard fires

		updateRegistryDirect(a, sharedID, highOffset, tsNew)
		t.Logf("after new-tailer update (offset=%s, ts=%d): committed=%s", highOffset, tsNew, committedOffset(a, sharedID))

		updateRegistryDirect(a, sharedID, lowOffset, tsOld)
		final := committedOffset(a, sharedID)
		t.Logf("after old-tailer update (offset=%s, ts=%d): committed=%s", lowOffset, tsOld, final)

		high, _ := strconv.ParseInt(highOffset, 10, 64)
		got, _ := strconv.ParseInt(final, 10, 64)

		if got < high {
			t.Fatalf(
				"UNEXPECTED FAILURE: guard failed to block old tailer (lower ts=%d).\n"+
					"  committed=%d, expected=%d\n"+
					"  This indicates updateRegistry guard is broken for all timestamp orderings.",
				tsOld, got, high,
			)
		}

		t.Logf("guard CORRECTLY blocked old-tailer update (stored ts=%d > incoming ts=%d).", tsNew, tsOld)
	})

	// Sub-test 4: normal monotonic operation — no collision, all updates accepted.
	//
	// Sequence of offset advances from a single logical tailer, each with a
	// strictly increasing timestamp. Every update should be accepted; the final
	// committed offset must equal the last one sent.
	//
	// EXPECTED TO PASS.
	t.Run("NewTailerRoundTripCorrect", func(t *testing.T) {
		a := buildAuditorForDemo(t)
		a.registry = make(map[string]*RegistryEntry)

		updates := []struct {
			offset string
			ts     int64
		}{
			{"100", 1},
			{"500", 2},
			{"1200", 3},
			{"3000", 4},
			{"5000", 5},
		}

		for _, u := range updates {
			updateRegistryDirect(a, sharedID, u.offset, u.ts)
			got := committedOffset(a, sharedID)
			if got != u.offset {
				t.Fatalf("monotonic update rejected unexpectedly: sent offset=%s ts=%d, committed=%s", u.offset, u.ts, got)
			}
		}

		final := committedOffset(a, sharedID)
		want := updates[len(updates)-1].offset
		if final != want {
			t.Fatalf("final offset mismatch: want=%s got=%s", want, final)
		}
		t.Logf("PASS: all %d monotonic updates accepted; final offset=%s", len(updates), final)
	})
}
