// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package ebpfless

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNanosToMicros(t *testing.T) {
	require.Equal(t, uint32(0), nanosToMicros(0))
	require.Equal(t, uint32(0), nanosToMicros(200))
	require.Equal(t, uint32(1), nanosToMicros(500))
	require.Equal(t, uint32(1), nanosToMicros(1000))
	require.Equal(t, uint32(1), nanosToMicros(1200))
	require.Equal(t, uint32(123), nanosToMicros(123*1000))
}

func TestSingleSampleRTT(t *testing.T) {
	var rt rttTracker

	require.False(t, rt.isActive())

	rt.processOutgoing(1000, 123)
	require.True(t, rt.isActive())

	hasUpdated := rt.processIncoming(2000, 42)
	// ack is too low, not a round trip
	require.False(t, hasUpdated)

	// ack is high enough to complete a round trip
	hasUpdated = rt.processIncoming(3000, 123)
	require.True(t, hasUpdated)

	require.Equal(t, uint64(2000), rt.rttSmoothNs)
	require.Equal(t, uint64(1000), rt.rttVarNs)
}

func TestLowVarianceRtt(t *testing.T) {
	var rt rttTracker

	for i := range 10 {
		ts := uint64(i + 1)
		seq := uint32(123 + i)

		startNs := (2 * ts) * 1000
		endNs := startNs + 1000
		// round trip time always the 1000, so variance goes to 0
		rt.processOutgoing(startNs, seq)
		hasUpdated := rt.processIncoming(endNs, seq)
		require.True(t, hasUpdated)
		require.Equal(t, rt.rttSmoothNs, uint64(1000))
	}

	// after 10 iterations, the variance should have mostly converged to zero
	require.Less(t, rt.rttVarNs, uint64(100))
}

func TestConstantVarianceRtt(t *testing.T) {
	var rt rttTracker

	for i := range 10 {
		ts := uint64(i + 1)
		seq := uint32(123 + i)

		startNs := (2 * ts) * 1000
		endNs := startNs + 500
		if i%2 == 0 {
			endNs = startNs + 1000
		}

		// round trip time alternates between 500 and 100
		rt.processOutgoing(startNs, seq)
		hasUpdated := rt.processIncoming(endNs, seq)
		require.True(t, hasUpdated)

		require.LessOrEqual(t, uint64(500), rt.rttSmoothNs)
		require.LessOrEqual(t, rt.rttSmoothNs, uint64(1000))
	}

	// This is not exact since it uses an exponential rolling sum
	// In this test, the time delta alternates between 500 and 1000,
	// so rttSmoothNs is 750, for an average difference of ~250.
	const epsilon = 20
	require.Less(t, uint64(250-epsilon), rt.rttVarNs)
	require.Less(t, rt.rttVarNs, uint64(250+epsilon))
}

func TestTcpProcessorRtt(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
	syn := pb.outgoing(0, 0, 0, SYN)
	// t=200 us
	syn.timestampNs = 200 * 1000
	synack := pb.incoming(0, 0, 1, SYN|ACK)
	// t=300 us, for a round trip of 100us
	synack.timestampNs = 300 * 1000

	f := newTCPTestFixture(t)

	f.runPkt(syn)
	// round trip has not completed yet
	require.Zero(t, f.conn.RTT)
	require.Zero(t, f.conn.RTTVar)

	f.runPkt(synack)
	// round trip has completed in 100us
	require.Equal(t, uint32(100), f.conn.RTT)
	require.Equal(t, uint32(50), f.conn.RTTVar)
}

func TestTcpProcessorRttRetransmit(t *testing.T) {
	pb := newPacketBuilder(lowerSeq, higherSeq)
	syn := pb.outgoing(0, 0, 0, SYN)
	// t=200 us
	syn.timestampNs = 200 * 1000
	synack := pb.incoming(0, 0, 1, SYN|ACK)
	// t=300 us, for a round trip of 100us
	synack.timestampNs = 300 * 1000

	f := newTCPTestFixture(t)

	f.runPkt(syn)
	// round trip has not completed yet
	require.Zero(t, f.conn.RTT)
	require.Zero(t, f.conn.RTTVar)

	f.runPkt(syn)
	// this is a retransmit, should reset the round trip
	require.Zero(t, f.conn.RTT)
	require.Zero(t, f.conn.RTTVar)

	f.runPkt(synack)
	// should STILL not have a round trip because the retransmit contaminated the results
	require.Zero(t, f.conn.RTT)
	require.Zero(t, f.conn.RTTVar)
}
