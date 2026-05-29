// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

package sources

// TestContainerAddRemoveSourceOrdering_OrphanedSource reproduces the
// container-addremovesource-ordering logical ordering hole deterministically.
//
// The bug: WrappedSource.Start() does `go Sources.AddSource(src)` and
// Stop() does `go Sources.RemoveSource(src)` — both fire-and-forget goroutines
// (pkg/logs/launchers/container/tailerfactory/tailers/source.go:34,42).
//
// For a container that starts and then immediately stops, the Go scheduler may
// run the Stop goroutine before the Start goroutine, producing the sequence:
//
//   1. RemoveSource(s)  — no-op because s is not yet in the list
//      (sources.go:87-94: sourceFound stays false, no subscriber notified)
//   2. AddSource(s)     — unconditionally appends s to s.sources (sources.go:56)
//
// After both operations, s is permanently present in LogSources even though
// the container has stopped. No subsequent RemoveSource will ever be called,
// so the source is orphaned: a launcher will discover it via GetSources() or
// a subscription and tail a dead container indefinitely.
//
// This test exercises that exact ordering directly — no goroutine scheduling
// tricks needed — and asserts the CORRECT post-condition: after a
// start-then-stop lifecycle, the source must NOT be present in LogSources.
// The test demonstrates the bug by showing the source IS present (orphaned).

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/stretchr/testify/assert"
)

func TestContainerAddRemoveSourceOrdering_OrphanedSource(t *testing.T) {
	// --- Setup ---
	ls := NewLogSources()
	src := NewLogSource("container-xyz", &config.LogsConfig{Type: "docker"})

	// --- Reproduce the race-produced ordering deterministically ---
	//
	// Normal (non-racy) order:  AddSource → RemoveSource → 0 sources
	// Bug-inducing (racy) order: RemoveSource → AddSource → 1 source (orphan)
	//
	// We call them in the racy order directly. This is exactly what happens
	// when the Go scheduler runs the Stop goroutine before the Start goroutine.

	// Step 1: Stop fires first — RemoveSource is a no-op (src not yet present).
	// sources.go:87-94: the loop finds nothing, sourceFound stays false,
	// no subscriber is notified, the slice is unchanged.
	ls.RemoveSource(src)

	sourcesAfterRemove := ls.GetSources()
	assert.Equal(t, 0, len(sourcesAfterRemove),
		"RemoveSource on absent source should be a no-op: 0 sources expected after remove-only")

	// Step 2: Start fires second — AddSource unconditionally appends to s.sources.
	// sources.go:56: `s.sources = append(s.sources, source)` runs regardless of
	// whether RemoveSource already ran. The source is now in the list permanently.
	ls.AddSource(src)

	sourcesAfterAdd := ls.GetSources()

	// CORRECT post-condition: a container that started and then immediately
	// stopped should leave ZERO sources in LogSources. The container is gone;
	// tailing it is incorrect.
	correctCount := 0

	// ACTUAL post-condition under the bug: 1 source remains (orphaned).
	// Any launcher calling GetSources() or receiving via a subscription will
	// find the dead container's source and begin (or continue) tailing it.
	buggyCount := 1

	orphanCount := len(sourcesAfterAdd)

	t.Logf("sources present after Remove-then-Add lifecycle: %d", orphanCount)
	t.Logf("correct (expected): %d — buggy (actual): %d", correctCount, buggyCount)

	if orphanCount == buggyCount {
		t.Logf("BUG REPRODUCED: source is ORPHANED in LogSources after container stop")
		t.Logf("  The racy Remove-before-Add ordering left src=%q permanently in the store.", src.Name)
		t.Logf("  LogSources.RemoveSource (sources.go:84-113) is a no-op when the source")
		t.Logf("  is absent; LogSources.AddSource (sources.go:53-78) always appends (line 56).")
		t.Logf("  WrappedSource.Start/Stop both use fire-and-forget goroutines (source.go:34,42),")
		t.Logf("  so this ordering is reachable in production under scheduler pressure.")
	}

	// Assert the CORRECT behaviour. This will FAIL (demonstrating the bug)
	// because orphanCount == 1, not 0.
	assert.Equal(t, correctCount, orphanCount,
		"FAIL: %d orphaned source(s) remain after a start-then-stop lifecycle; "+
			"expected 0. The racy Remove-before-Add ordering (source.go:34,42) leaves "+
			"the source permanently in LogSources (sources.go:56 always appends).",
		orphanCount)
}

// TestContainerAddRemoveSourceOrdering_NormalOrder verifies the happy path:
// when AddSource runs before RemoveSource (the intended ordering), no source
// is left behind. This is the non-racy case that already works correctly.
func TestContainerAddRemoveSourceOrdering_NormalOrder(t *testing.T) {
	ls := NewLogSources()
	src := NewLogSource("container-xyz", &config.LogsConfig{Type: "docker"})

	// Normal order: Add then Remove.
	ls.AddSource(src)
	assert.Equal(t, 1, len(ls.GetSources()), "source should be present after AddSource")

	ls.RemoveSource(src)
	assert.Equal(t, 0, len(ls.GetSources()),
		"source should be absent after RemoveSource: correct normal-order lifecycle")

	t.Logf("Normal order (Add-then-Remove): %d sources remain — correct", len(ls.GetSources()))
}
