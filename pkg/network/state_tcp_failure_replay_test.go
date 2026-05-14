// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package network

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// Helpers duplicated from state_test.go because that file is gated to
// //go:build linux || windows. We want this reproducer runnable on macOS too.
var replayEpoch atomic.Uint64

func replayLatestEpochTime() uint64 { return replayEpoch.Add(1) }

func replayNewDefaultState() *networkState {
	return NewState(nil, 2*time.Minute, 50000, 75000, 75000, 7500, 7500, 7500, 7500, false, false, []int{53}).(*networkState)
}

// TestWindowsTCPFailureReplay verifies the agent-side fix for the >100% TCP
// failure rate seen on Windows (lingering driver flows).
//
// Root cause: the ddnpm driver keeps a flow with a terminal ConnectionStatus
// (e.g. RecvRst) in its open-flows table across multiple agent polls when a
// concurrent WFP transport-callout holds the flow's refcount above 2 at the
// moment of closure. PR #47005 ensures FlowToConnStat sets Monotonic.TCPClosed=1
// when TCPFailures is non-empty, but the flows stay in open-flows and keep
// appearing every poll, each time with a fresh TCPFailures={104:1} value that
// has no baseline in StatCounters — so the failures accumulate while the closed
// delta is 0 after poll 1.
//
// The agent fix (updateConnWithStats): when a connection was already reported
// as closed (sts.TCPClosed>0) and no new close event exists (last.TCPClosed==0),
// nil out TCPFailures so the re-report ships nothing. This prevents the failure
// count from growing past the closed count and keeps the backend rate at ≤100%.
func TestWindowsTCPFailureReplay(t *testing.T) {
	// Build a ConnectionStats matching what FlowToConnStat (event_windows.go:131)
	// produces for a flow whose driver.ConnectionStatus == ConnectionStatusRecvRst,
	// post PR #47005 (which sets Monotonic.TCPClosed = 1 whenever len(TCPFailures) > 0).
	// Windows agent hardcodes the POSIX value 104 (see event_windows.go:146)
	// regardless of host platform.
	const eConnReset = uint16(104)

	// monotonicallyIncreasingBytes simulates the realistic case where the
	// driver-reported flow still has some byte traffic across polls — otherwise
	// the state machine filters out all-zero deltas at state.go:722 and the
	// connection would be absent from the delta entirely.
	makeFailedFlow := func(pollIdx int) ConnectionStats {
		return ConnectionStats{
			ConnectionTuple: ConnectionTuple{
				Pid:    1234,
				Type:   TCP,
				Family: AFINET,
				Source: util.AddressFromString("10.0.0.1"),
				Dest:   util.AddressFromString("10.0.0.2"),
				SPort:  45678,
				DPort:  443,
			},
			Monotonic: StatCounters{
				SentBytes:      uint64(1500 * pollIdx), // grows each poll
				RecvBytes:      uint64(200 * pollIdx),
				TCPEstablished: 1,
				TCPClosed:      1, // PR #47005: set because TCPFailures is non-empty
			},
			TCPFailures: map[uint16]uint32{
				eConnReset: 1, // ConnectionStatusRecvRst -> ECONNRESET (event_windows.go:146)
			},
			Cookie: 0xCAFEBABE,
		}
	}

	const clientID = "test-client"
	state := replayNewDefaultState()
	state.RegisterClient(clientID)

	// Record the lingering-flows counter before the test so we can assert it
	// increments exactly once per suppressed re-report.
	lingeringBefore := stateTelemetry.windowsLingeringFlows.Load()

	const numPolls = 3
	var (
		totalFailuresShipped uint32
		totalClosedShipped   uint32
	)

	for poll := 1; poll <= numPolls; poll++ {
		conn := makeFailedFlow(poll)
		delta := state.GetDelta(clientID, replayLatestEpochTime(), []ConnectionStats{conn}, nil, nil)
		require.Len(t, delta.Conns, 1, "poll %d should report 1 connection", poll)

		shipped := delta.Conns[0]
		lastClosed := uint32(shipped.Last.TCPClosed)
		var failuresThisPoll uint32
		for _, v := range shipped.TCPFailures {
			failuresThisPoll += v
		}

		t.Logf("poll=%d  Last.TCPClosed=%d  TCPFailures=%v", poll, lastClosed, shipped.TCPFailures)

		totalClosedShipped += lastClosed
		totalFailuresShipped += failuresThisPoll

		switch poll {
		case 1:
			// First poll: no prior state, Last = Monotonic. Both closed and failure
			// are reported. Backend ratio at this point = 1/1 = 100%.
			require.Equal(t, uint32(1), lastClosed, "poll 1 must ship TCPClosed=1")
			require.Equal(t, uint32(1), failuresThisPoll, "poll 1 must ship 1 failure")
		default:
			// Subsequent polls: the fix suppresses TCPFailures for a flow that was
			// already reported as closed. TCPClosed delta is 0 (already counted).
			// The connection still appears because SentBytes delta is non-zero.
			require.Equal(t, uint32(0), lastClosed,
				"poll %d: TCPClosed delta is 0 (already counted on poll 1)", poll)
			require.Equal(t, uint32(0), failuresThisPoll,
				"poll %d: failures suppressed by lingering-flow fix — must not accumulate", poll)
		}
	}

	// Overall: only one closed and one failure ever ship.
	backendRatePct := float64(totalFailuresShipped) * 100.0 / float64(totalClosedShipped)
	t.Logf("=== aggregated across %d polls ===", numPolls)
	t.Logf("sum(TCPFailures) = %d", totalFailuresShipped)
	t.Logf("sum(TCPClosed)   = %d", totalClosedShipped)
	t.Logf("backend rate     = %.0f%%", backendRatePct)

	require.Equal(t, uint32(1), totalFailuresShipped, "only poll 1 ships a failure; re-reports are suppressed")
	require.Equal(t, uint32(1), totalClosedShipped, "TCPClosed only shipped on first poll")
	require.Equal(t, 100.0, backendRatePct, "fix keeps backend rate at exactly 100%%")

	// The telemetry counter must have incremented for each suppressed re-report.
	lingeringAfter := stateTelemetry.windowsLingeringFlows.Load()
	require.Equal(t, int64(numPolls-1), lingeringAfter-lingeringBefore,
		"windowsLingeringFlows counter should increment once per suppressed poll")
}

// TestWindowsTCPFailureReplay_ProposedFix shows that gating failure emission
// on Last.TCPClosed > 0 (the proposed format.go:116 patch) keeps the rate at 100%.
//
// This is what would change in pkg/network/encoding/marshal/format.go:
//
//	if len(conn.TCPFailures) > 0 && conn.Last.TCPClosed > 0 {
//	    builder.AddTcpFailuresByErrCode(...)
//	}
func TestWindowsTCPFailureReplay_ProposedFix(t *testing.T) {
	const eConnReset = uint16(104)

	makeFailedFlow := func(pollIdx int) ConnectionStats {
		return ConnectionStats{
			ConnectionTuple: ConnectionTuple{
				Pid: 1234, Type: TCP, Family: AFINET,
				Source: util.AddressFromString("10.0.0.1"),
				Dest:   util.AddressFromString("10.0.0.2"),
				SPort:  45678, DPort: 443,
			},
			Monotonic: StatCounters{
				TCPEstablished: 1,
				TCPClosed:      1,
				SentBytes:      uint64(1500 * pollIdx),
			},
			TCPFailures: map[uint16]uint32{eConnReset: 1},
			Cookie:      0xCAFEBABE,
		}
	}

	const clientID = "test-client-fixed"
	state := replayNewDefaultState()
	state.RegisterClient(clientID)

	// Emulate the proposed encoder gate.
	encodeWithFix := func(c ConnectionStats) (lastClosed, failures uint32) {
		lastClosed = uint32(c.Last.TCPClosed)
		if c.Last.TCPClosed > 0 { // <-- the proposed gate
			for _, v := range c.TCPFailures {
				failures += v
			}
		}
		return
	}

	var sumFailures, sumClosed uint32
	for poll := 1; poll <= 5; poll++ {
		conn := makeFailedFlow(poll)
		delta := state.GetDelta(clientID, replayLatestEpochTime(), []ConnectionStats{conn}, nil, nil)
		require.Len(t, delta.Conns, 1)

		closed, failures := encodeWithFix(delta.Conns[0])
		t.Logf("poll=%d  shipped: closed=%d failures=%d", poll, closed, failures)
		sumClosed += closed
		sumFailures += failures
	}

	require.Equal(t, uint32(1), sumClosed, "TCPClosed shipped exactly once (first poll)")
	require.Equal(t, uint32(1), sumFailures, "failure shipped exactly once (alongside the close)")
	require.Equal(t, 100.0, float64(sumFailures)*100.0/float64(sumClosed),
		"with the fix, rate stays at 100%%")
}

// Run with: dda inv test --targets=./pkg/network -- -run TestWindowsTCPFailureReplay -v
