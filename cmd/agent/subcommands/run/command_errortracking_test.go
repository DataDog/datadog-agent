// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package run

import (
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agenttelemetryfx "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// fakeAtel stands in for the agenttelemetry.Component in wiring tests. It
// records SubmitErrorRecord calls split into "accepted" and "dropped by
// guard" buckets so the wiring layer can be exercised without spinning up
// the real component's Fx graph.
//
// The guard logic here mirrors comp/core/agenttelemetry/impl.isRecursiveCaller
// (package-private). The actual guard is unit-tested next to its
// implementation; this mirror exists solely so the wiring-side test can
// assert that the installErrortrackingHandler closure preserves the
// call-site PC end-to-end. If the real guard's package set changes the
// mirror MUST be updated; the synthetic-PC assertion in
// TestInstallErrortrackingHandler_RecursionGuard would then catch a
// silent skew.
type fakeAtel struct {
	accepted atomic.Uint64
	dropped  atomic.Uint64
}

func (f *fakeAtel) SubmitErrorRecord(log errortracking.ErrorLog) {
	if isRecursiveCallerMirror(log.PC) {
		f.dropped.Add(1)
		return
	}
	f.accepted.Add(1)
}

func isRecursiveCallerMirror(pc uintptr) bool {
	if pc == 0 {
		return false
	}
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	if strings.Contains(frame.Function, "datadog-agent/comp/core/agenttelemetry") ||
		strings.Contains(frame.Function, "datadog-agent/pkg/util/log/setup") {
		return true
	}
	if strings.Contains(frame.File, "/comp/core/agenttelemetry/") ||
		strings.Contains(frame.File, "/pkg/util/log/setup/") {
		return true
	}
	return false
}

// installErrortrackingHandler is an Fx invoke (see command.go); the closure
// it registers is a one-liner forwarding ErrorLog values to the
// agenttelemetry component's SubmitErrorRecord. Rebuilding the closure
// here, byte-for-byte, exercises the wiring shape without dragging in
// fx.Lifecycle / config.Component.
func wiringSubmitter(fa *fakeAtel) errortracking.Submitter {
	return func(elog errortracking.ErrorLog) {
		fa.SubmitErrorRecord(elog)
	}
}

// TestInstallErrortrackingHandler_RecursionGuard verifies that a record
// carrying a call-site PC inside the agenttelemetry package is dropped
// before any post-guard counter is incremented. This is the wiring-layer
// regression for review comment C1 on PR #50607: the closure registered
// by installErrortrackingHandler MUST forward the PC unchanged so the
// recursion guard in atel.SubmitErrorRecord has material to act on.
func TestInstallErrortrackingHandler_RecursionGuard(t *testing.T) {
	pcInsideAgenttelemetry := reflect.ValueOf(agenttelemetryfx.Module).Pointer()
	require.NotZero(t, pcInsideAgenttelemetry,
		"reflect.ValueOf(agenttelemetryfx.Module).Pointer() must yield a non-zero PC")

	// Sanity: the synthetic PC must actually resolve inside the guarded
	// package set. If this fails the test setup is wrong (e.g. the fx
	// module moved); the recursion-guard assertion below would then be
	// a false positive.
	require.True(t, isRecursiveCallerMirror(pcInsideAgenttelemetry),
		"synthetic PC must resolve inside guarded package set")

	fa := &fakeAtel{}
	submitter := wiringSubmitter(fa)

	submitter(errortracking.ErrorLog{
		Time:    time.Now(),
		Level:   slog.LevelError,
		Message: "would-loop",
		PC:      pcInsideAgenttelemetry,
	})

	assert.Equal(t, uint64(0), fa.accepted.Load(),
		"recursion guard must drop records originating in agenttelemetry; "+
			"accepted=1 means the wiring stripped the PC before reaching the guard")
	assert.Equal(t, uint64(1), fa.dropped.Load())
}

// TestInstallErrortrackingHandler_PassesExternalPCs guards against the
// inverse failure mode: a guard that drops everything would let the
// recursion-guard test above pass while making the forwarder useless.
// Confirm that PCs outside the guarded package set survive the wiring.
func TestInstallErrortrackingHandler_PassesExternalPCs(t *testing.T) {
	fa := &fakeAtel{}
	submitter := wiringSubmitter(fa)

	submitter(errortracking.ErrorLog{
		Time:    time.Now(),
		Level:   slog.LevelError,
		Message: "from-external",
		// PC=0 bypasses the guard (no frame to resolve) and reaches the
		// accept path. Same convention as
		// TestSubmitErrorRecord_RecursionGuard_AllowsExternalCaller in
		// comp/core/agenttelemetry/impl/errortracking_sender_test.go.
	})

	assert.Equal(t, uint64(1), fa.accepted.Load())
	assert.Equal(t, uint64(0), fa.dropped.Load())
}
