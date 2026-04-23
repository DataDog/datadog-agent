// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build docker

package docker

// Tests for the panic-recovery wrapper used inside the collector's stream
// goroutine. See taskmds/05001 for the underlying bug that motivated adding
// this wrapper.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRunWithRecovery_SwallowsPanic verifies that runWithRecovery swallows
// a panic raised inside the passed function and returns normally. This is
// the property the docker workloadmeta collector's stream goroutine relies
// on — without it, a panic inside ContainerInspect (see taskmds/05001)
// propagates up to the goroutine and kills the whole agent process.
func TestRunWithRecovery_SwallowsPanic(t *testing.T) {
	didReturn := false

	runWithRecovery("unit test", func() {
		panic("simulated moby/client json decode panic")
	})

	didReturn = true
	require.Truef(t, didReturn,
		"runWithRecovery must return to its caller even when the wrapped "+
			"function panics; if this assertion fires it means the defer/recover "+
			"pair was removed and any panic inside handleContainerEvent will "+
			"tear down the collector goroutine (and in production, the agent "+
			"process itself).")
}

// TestRunWithRecovery_NoPanicNoOp verifies the happy path: if the wrapped
// function returns normally, runWithRecovery does not alter control flow.
func TestRunWithRecovery_NoPanicNoOp(t *testing.T) {
	callCount := 0
	runWithRecovery("unit test", func() {
		callCount++
	})
	require.Equal(t, 1, callCount,
		"runWithRecovery must invoke the wrapped function exactly once "+
			"when it returns normally")
}

// TestRunWithRecovery_SubsequentCallsAfterPanic is the behaviour the stream
// loop cares about: after one call to runWithRecovery panics, subsequent
// calls must still work. This is what makes a single bad container event
// safe to drop without taking the collector with it.
func TestRunWithRecovery_SubsequentCallsAfterPanic(t *testing.T) {
	runs := 0

	runWithRecovery("first", func() {
		runs++
		panic("first call panics")
	})
	runWithRecovery("second", func() {
		runs++
	})
	runWithRecovery("third", func() {
		runs++
		panic("third call panics too")
	})
	runWithRecovery("fourth", func() {
		runs++
	})

	require.Equal(t, 4, runs,
		"runWithRecovery must be reusable across events — each call is "+
			"independent; a panic in one call must not affect the next")
}
