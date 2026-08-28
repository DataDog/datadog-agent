// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/libproc"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
	processutil "github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestDarwinLibprocReconcilerScansImmediately(_ *testing.T) {
	scanner := &signalingLibprocScanner{scanned: make(chan struct{})}
	reconciler := newDarwinLibprocReconciler(
		scanner,
		newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl()),
		time.Hour,
	)

	reconciler.start()
	<-scanner.scanned
	reconciler.stop()
}

func TestDarwinLibprocReconcilerFillsOnlyUnresolvedPID(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatTCPFlow(0, tcpStateEstablished)
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      flow,
	})

	resolved, ambiguous, reuseRejected := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{testDarwinLibprocObservation(1234, 1)},
	})

	require.Equal(t, 1, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, uint32(1234), buffer.Connections()[0].Pid)
	require.Equal(t, flow.Local.Address, buffer.Connections()[0].Source.Addr)
	require.Equal(t, flow.Remote.Address, buffer.Connections()[0].Dest.Addr)
}

func TestDarwinLibprocReconcilerRejectsAmbiguousOwnership(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(0, tcpStateEstablished),
	})
	first := testDarwinLibprocObservation(1234, 1)
	second := testDarwinLibprocObservation(5678, 2)

	resolved, ambiguous, _ := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{first, second},
	})

	require.Zero(t, resolved)
	require.Equal(t, 1, ambiguous)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Zero(t, buffer.Connections()[0].Pid)
}

func TestDarwinLibprocReconcilerRejectsCandidateWithWrongPID(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatTCPFlow(1234, tcpStateEstablished)
	flow.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      flow,
	})

	resolved, ambiguous, reuseRejected := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{testDarwinLibprocObservation(5678, 1)},
	})

	require.Zero(t, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	require.Equal(t, uint32(1234), tracer.sources[1].flow.PID)
	require.False(t, nstatEndpointComplete(tracer.sources[1].flow.Remote))
}

func TestDarwinLibprocReconcilerSelectsCandidateWithAuthoritativePID(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatTCPFlow(1234, tcpStateEstablished)
	flow.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      flow,
	})
	correct := testDarwinLibprocObservation(1234, 1)
	wrong := testDarwinLibprocObservation(5678, 1)
	wrong.Tuple.Dest.Addr = netip.MustParseAddr("198.51.100.21")
	wrong.Tuple.DPort = 8443

	resolved, ambiguous, reuseRejected := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{wrong, correct},
	})

	require.Equal(t, 1, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, uint32(1234), buffer.Connections()[0].Pid)
	require.Equal(t, correct.Tuple.Dest.Addr, buffer.Connections()[0].Dest.Addr)
	require.Equal(t, correct.Tuple.DPort, buffer.Connections()[0].DPort)
}

func TestDarwinLibprocReconcilerRejectsAmbiguousSocketsFromSameProcess(t *testing.T) {
	first := testDarwinLibprocObservation(1234, 1)
	second := first
	second.Tuple.Dest.Addr = netip.MustParseAddr("198.51.100.21")
	second.Tuple.DPort = 8443

	for name, observations := range map[string][]libproc.Observation{
		"forward scan order": {first, second},
		"reverse scan order": {second, first},
	} {
		t.Run(name, func(t *testing.T) {
			tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
			flow := testNStatTCPFlow(0, tcpStateEstablished)
			flow.Remote = nstat.Endpoint{}
			tracer.processEvent(nstat.Event{
				Kind:      nstat.EventDescription,
				SourceRef: 1,
				Provider:  nstat.ProviderTCPKernel,
				Flow:      flow,
			})

			resolved, ambiguous, reuseRejected := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
				Observations: observations,
			})

			require.Zero(t, resolved)
			require.Equal(t, 1, ambiguous)
			require.Zero(t, reuseRejected)
			require.False(t, nstatEndpointComplete(tracer.sources[1].flow.Remote))
		})
	}
}

func TestDarwinLibprocReconcilerDeduplicatesIdenticalSocketObservations(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatTCPFlow(0, tcpStateEstablished)
	flow.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      flow,
	})
	observation := testDarwinLibprocObservation(1234, 1)

	resolved, ambiguous, reuseRejected := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{observation, observation},
	})

	require.Equal(t, 1, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, observation.Tuple.Dest.Addr, buffer.Connections()[0].Dest.Addr)
	require.Equal(t, observation.Tuple.DPort, buffer.Connections()[0].DPort)
}

func TestDarwinLibprocEvidenceCannotOverwriteNStatTuple(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatTCPFlow(0, tcpStateEstablished)
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      flow,
	})
	conflicting := testDarwinLibprocObservation(1234, 1)
	conflicting.Tuple.Source.Addr = netip.MustParseAddr("203.0.113.10")
	conflicting.Tuple.Dest.Addr = netip.MustParseAddr("203.0.113.20")
	conflicting.Tuple.SPort = 1234
	conflicting.Tuple.DPort = 4321

	tracer.mu.Lock()
	tracer.applyLibprocEvidence(1, tracer.sources[1], conflicting)
	tracer.mu.Unlock()

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	conn := buffer.Connections()[0]
	require.Equal(t, uint32(1234), conn.Pid)
	require.Equal(t, flow.Local.Address, conn.Source.Addr)
	require.Equal(t, flow.Local.Port, conn.SPort)
	require.Equal(t, flow.Remote.Address, conn.Dest.Addr)
	require.Equal(t, flow.Remote.Port, conn.DPort)
	require.Equal(t, uint32(tcpStateEstablished), tracer.sources[1].flow.TCPState)
}

func TestDarwinLibprocReconcilerCompletesUDPConnectionTuple(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatUDPFlow(0)
	flow.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      flow,
	})
	observation := testDarwinLibprocObservation(1234, 1)
	observation.Tuple.Type = network.UDP

	resolved, ambiguous, reuseRejected := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{observation},
	})

	require.Equal(t, 1, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	conn := buffer.Connections()[0]
	require.Equal(t, observation.Tuple.Dest.Addr, conn.Dest.Addr)
	require.Equal(t, observation.Tuple.DPort, conn.DPort)
	require.Equal(t, uint32(1234), conn.Pid)
	match := tracer.tuples.match(conn.ConnectionTuple)
	require.True(t, match.matched)
	require.Equal(t, conn.Cookie, match.cookie)
}

func TestDarwinLibprocReconcilerIgnoresUnconnectedUDPWithNStatPID(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	flow := testNStatUDPFlow(1234)
	flow.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      flow,
	})
	observation := testDarwinLibprocObservation(9876, 1)
	observation.Tuple.Type = network.UDP

	resolved, ambiguous, reuseRejected := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{observation},
	})

	require.Zero(t, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	conn := buffer.Connections()[0]
	require.Equal(t, uint32(1234), conn.Pid)
	require.False(t, conn.Dest.Addr.IsValid())
	require.Zero(t, conn.DPort)
}

func TestDarwinLibprocReconcilerRejectsPIDReuse(t *testing.T) {
	now := time.Unix(100, 0)
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.now = func() time.Time { return now }
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(0, tcpStateEstablished),
	})
	observation := testDarwinLibprocObservation(1234, uint64(now.Add(2*time.Second).UnixNano()))

	resolved, _, reuseRejected := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{observation},
	})

	require.Zero(t, resolved)
	require.Equal(t, 1, reuseRejected)
}

func TestDarwinLibprocReconcilerCompletesRemovedPartialSourceOnce(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	partial := testNStatTCPFlow(0, tcpStateEstablished)
	partial.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 1,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      partial,
	})
	tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 1})
	var closed []*network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) {
		closed = append(closed, conn)
	}

	resolved, ambiguous, reuseRejected := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{testDarwinLibprocObservation(1234, 1)},
	})

	require.Equal(t, 1, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	require.Len(t, closed, 1)
	require.True(t, closed[0].IsClosed)
	require.Equal(t, uint32(1234), closed[0].Pid)

	tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{testDarwinLibprocObservation(1234, 1)},
	})
	require.Len(t, closed, 1)
}

func testDarwinLibprocObservation(pid uint32, start uint64) libproc.Observation {
	return libproc.Observation{
		Tuple: network.ConnectionTuple{
			Source: processutil.Address{Addr: netip.MustParseAddr("192.0.2.10")},
			Dest:   processutil.Address{Addr: netip.MustParseAddr("198.51.100.20")},
			SPort:  50000,
			DPort:  443,
			Type:   network.TCP,
			Family: network.AFINET,
		},
		PID:              pid,
		ProcessStartTime: start,
	}
}

type signalingLibprocScanner struct {
	once    sync.Once
	scanned chan struct{}
}

func (s *signalingLibprocScanner) Scan() (libproc.Snapshot, error) {
	s.once.Do(func() { close(s.scanned) })
	return libproc.Snapshot{}, nil
}
