// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package network

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func replayNewDefaultState() *networkState {
	return NewState(nil, 2*time.Minute, 50000, 75000, 75000, 7500, 7500, 7500, 7500, false, false, []int{53}).(*networkState)
}

// lingeringFlowFixture returns a ConnectionStats that models what FlowToConnStat
// produces for a Windows flow stuck in the driver's openFlows table.
// pollIdx drives monotonically increasing byte counts so the delta is non-zero
// and the connection is not filtered before updateConnWithStats runs.
func lingeringFlowFixture(pollIdx int) ConnectionStats {
	// 104 matches what event_windows.go:146 emits for ConnectionStatusRecvRst.
	const eConnReset = uint16(104)
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
			SentBytes:      uint64(1500 * pollIdx),
			RecvBytes:      uint64(200 * pollIdx),
			TCPEstablished: 1,
			TCPClosed:      1, // PR #47005: set because TCPFailures is non-empty
		},
		TCPFailures: map[uint16]uint32{eConnReset: 1},
		Cookie:      0xCAFEBABE,
	}
}

// TestWindowsTCPFailureReplay verifies the agent-side fix for the >100% TCP
// failure rate caused by flows stuck in the Windows ddnpm driver's openFlows
// table (lingering-flow bug, WINA-2711).
//
// The fix lives in state_windows.go: when a connection was already reported as
// closed (sts.TCPClosed > 0) and the current delta has no new close
// (last.TCPClosed == 0), zero out TCPFailures so no additional failures ship.
func TestWindowsTCPFailureReplay(t *testing.T) {
	var epoch uint64
	nextEpoch := func() uint64 { epoch++; return epoch }

	const clientID = "test-client"
	state := replayNewDefaultState()
	state.RegisterClient(clientID)

	lingeringBefore := stateTelemetry.windowsLingeringFlows.Load()

	const numPolls = 3
	var totalFailures, totalClosed uint32

	for poll := 1; poll <= numPolls; poll++ {
		poll := poll
		t.Run(fmt.Sprintf("poll=%d", poll), func(t *testing.T) {
			conn := lingeringFlowFixture(poll)
			delta := state.GetDelta(clientID, nextEpoch(), []ConnectionStats{conn}, nil, nil)
			require.Len(t, delta.Conns, 1, "poll %d should report 1 connection", poll)

			shipped := delta.Conns[0]
			lastClosed := uint32(shipped.Last.TCPClosed)
			var failuresThisPoll uint32
			for _, v := range shipped.TCPFailures {
				failuresThisPoll += v
			}
			t.Logf("Last.TCPClosed=%d TCPFailures=%v", lastClosed, shipped.TCPFailures)

			totalClosed += lastClosed
			totalFailures += failuresThisPoll

			switch poll {
			case 1:
				require.Equal(t, uint32(1), lastClosed, "poll 1 must ship TCPClosed=1")
				require.Equal(t, uint32(1), failuresThisPoll, "poll 1 must ship 1 failure")
			default:
				require.Equal(t, uint32(0), lastClosed,
					"poll %d: TCPClosed delta is 0 (already counted on poll 1)", poll)
				require.Equal(t, uint32(0), failuresThisPoll,
					"poll %d: failures suppressed by lingering-flow fix", poll)
			}
		})
	}

	require.Equal(t, uint32(1), totalFailures, "only poll 1 ships a failure; re-reports are suppressed")
	require.Equal(t, uint32(1), totalClosed, "TCPClosed only shipped on first poll")

	require.NotZero(t, totalClosed, "GetDelta never shipped a TCPClosed event across all polls")
	backendRatePct := float64(totalFailures) * 100.0 / float64(totalClosed)
	require.Equal(t, 100.0, backendRatePct, "fix keeps backend rate at exactly 100%%")

	lingeringAfter := stateTelemetry.windowsLingeringFlows.Load()
	require.Equal(t, int64(numPolls-1), lingeringAfter-lingeringBefore,
		"windowsLingeringFlows counter should increment once per suppressed re-report")
}

// TestWindowsTCPFailureReplay_NoFailuresNoOp verifies that the suppression path
// is a no-op for flows with empty TCPFailures.
func TestWindowsTCPFailureReplay_NoFailuresNoOp(t *testing.T) {
	var epoch uint64
	nextEpoch := func() uint64 { epoch++; return epoch }

	const clientID = "test-nofailures"
	state := replayNewDefaultState()
	state.RegisterClient(clientID)

	lingeringBefore := stateTelemetry.windowsLingeringFlows.Load()

	for poll := 1; poll <= 2; poll++ {
		conn := ConnectionStats{
			ConnectionTuple: ConnectionTuple{Pid: 1, Type: TCP, Family: AFINET,
				Source: util.AddressFromString("10.0.0.3"), Dest: util.AddressFromString("10.0.0.4"),
				SPort: 2222, DPort: 80},
			Monotonic: StatCounters{SentBytes: uint64(1000 * poll), TCPClosed: 1},
			Cookie:    0xBEEFCAFE,
		}
		delta := state.GetDelta(clientID, nextEpoch(), []ConnectionStats{conn}, nil, nil)
		require.Len(t, delta.Conns, 1, "poll %d", poll)
	}

	require.Equal(t, lingeringBefore, stateTelemetry.windowsLingeringFlows.Load(),
		"counter must not increment for flows with no TCPFailures")
}
