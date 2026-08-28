// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package connection

import (
	"errors"
	"io"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/libproc"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/nstat"
)

func TestNStatTracerTCPActiveAndFinalLifecycle(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(100, 0)
	tracer.now = func() time.Time { return now }
	var closed []*network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) {
		closed = append(closed, conn)
	}

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventAdded,
		SourceRef: 7,
		Provider:  nstat.ProviderTCPKernel,
	})
	tracer.pumpDescriptionRequests(now)
	require.Equal(t, []uint64{7}, control.descriptions())

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 7,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(1234, tcpStateSynSent),
	})
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventUpdate,
		SourceRef: 7,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(1234, tcpStateEstablished),
		Counts: &nstat.Counts{
			RXPackets:            11,
			RXBytes:              1200,
			TXPackets:            12,
			TXBytes:              1417,
			TXRetransmittedBytes: 17,
			AverageRTT:           905,
			RTTVariance:          32,
			ConnectAttempts:      1,
			ConnectSuccesses:     1,
		},
	})

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	active := buffer.Connections()
	require.Len(t, active, 1)
	require.Equal(t, uint32(1234), active[0].Pid)
	require.Equal(t, uint64(1400), active[0].Monotonic.SentBytes)
	require.Equal(t, uint64(1200), active[0].Monotonic.RecvBytes)
	require.Equal(t, uint16(1), active[0].Monotonic.TCPEstablished)
	require.Equal(t, uint64(12), active[0].Monotonic.SentPackets)
	require.Equal(t, uint64(11), active[0].Monotonic.RecvPackets)
	require.Equal(t, uint32(28281), active[0].RTT)
	require.Equal(t, uint32(2000), active[0].RTTVar)
	require.Equal(t, uint32(14), active[0].InterfaceIndex)
	require.Equal(t, network.OUTGOING, active[0].Direction)
	require.Equal(t, directionEvidenceTCPState, tracer.sources[7].directionEvidence)
	require.False(t, active[0].IsClosed)

	now = now.Add(3 * time.Second)
	tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 7})
	tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 7})

	require.Len(t, closed, 1)
	require.True(t, closed[0].IsClosed)
	require.Equal(t, uint16(1), closed[0].Monotonic.TCPClosed)
	require.Equal(t, 3*time.Second, closed[0].Duration)
	buffer.Reset()
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Empty(t, buffer.Connections())
}

func TestNStatTracerDoesNotCountPreexistingTCPConnectionAsEstablished(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 17,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(1234, tcpStateEstablished),
	})
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventUpdate,
		SourceRef: 17,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(1234, tcpStateEstablished+1),
	})

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Zero(t, buffer.Connections()[0].Monotonic.TCPEstablished)
	require.Equal(t, network.UNKNOWN, buffer.Connections()[0].Direction)
	require.Equal(t, directionEvidenceNone, tracer.sources[17].directionEvidence)
}

func TestNStatTracerInfersIncomingFromSynReceived(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 18,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(4321, tcpStateSynReceived),
	})

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, network.INCOMING, buffer.Connections()[0].Direction)
	require.Equal(t, directionEvidenceTCPState, tracer.sources[18].directionEvidence)
}

func TestNStatTracerInfersFastLoopbackPairWithoutSynStates(t *testing.T) {
	for _, tc := range []struct {
		name       string
		clientFlow *nstat.Flow
		serverFlow *nstat.Flow
	}{
		{
			name:       "ipv4",
			clientFlow: testNStatLoopbackTCPFlow(1001, "127.0.0.1", 50000, 8080),
			serverFlow: testNStatLoopbackTCPFlow(1002, "127.0.0.1", 8080, 50000),
		},
		{
			name:       "ipv6",
			clientFlow: testNStatLoopbackTCPFlow(2001, "::1", 50001, 8081),
			serverFlow: testNStatLoopbackTCPFlow(2002, "::1", 8081, 50001),
		},
	} {
		for _, listenerFirst := range []bool{true, false} {
			for _, serverFirst := range []bool{true, false} {
				order := "client_first"
				if serverFirst {
					order = "server_first"
				}
				listenerOrder := "listener_late"
				if listenerFirst {
					listenerOrder = "listener_first"
				}
				t.Run(tc.name+"/"+listenerOrder+"/"+order, func(t *testing.T) {
					tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
					listener := nstat.Event{
						Kind:      nstat.EventDescription,
						SourceRef: 20,
						Provider:  nstat.ProviderTCPKernel,
						Flow: testNStatTCPListenerFlow(
							tc.serverFlow.PID,
							tc.serverFlow.Local.Address.String(),
							tc.serverFlow.Local.Port,
						),
					}
					client := nstat.Event{
						Kind:      nstat.EventDescription,
						SourceRef: 21,
						Provider:  nstat.ProviderTCPKernel,
						Flow:      tc.clientFlow,
					}
					server := nstat.Event{
						Kind:      nstat.EventDescription,
						SourceRef: 22,
						Provider:  nstat.ProviderTCPKernel,
						Flow:      tc.serverFlow,
					}
					if listenerFirst {
						tracer.processEvent(listener)
					}
					if serverFirst {
						tracer.processEvent(server)
						tracer.processEvent(client)
					} else {
						tracer.processEvent(client)
						tracer.processEvent(server)
					}
					if !listenerFirst {
						tracer.processEvent(listener)
					}

					var buffer network.ConnectionBuffer
					require.NoError(t, tracer.GetConnections(&buffer, nil))
					require.Len(t, buffer.Connections(), 2)
					byPID := make(map[uint32]network.ConnectionStats, 2)
					for _, conn := range buffer.Connections() {
						byPID[conn.Pid] = conn
					}
					clientConn, found := byPID[tc.clientFlow.PID]
					require.True(t, found)
					serverConn, found := byPID[tc.serverFlow.PID]
					require.True(t, found)
					require.Equal(t, network.OUTGOING, clientConn.Direction)
					require.Equal(t, network.INCOMING, serverConn.Direction)
					require.Equal(t, directionEvidencePeer, tracer.sources[21].directionEvidence)
					require.Equal(t, directionEvidenceListener, tracer.sources[22].directionEvidence)
				})
			}
		}
	}
}

func TestNStatTracerDirectionEvidencePrecedence(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 23,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(3001, tcpStateSynReceived),
	})
	source := tracer.sources[23]
	require.Equal(t, network.INCOMING, source.conn.Direction)
	require.Equal(t, directionEvidenceTCPState, source.directionEvidence)

	tracer.setSourceDirection(source, network.OUTGOING, directionEvidenceListener)
	require.Equal(t, network.INCOMING, source.conn.Direction)
	require.Equal(t, directionEvidenceTCPState, source.directionEvidence)

	tracer.setSourceDirection(source, network.OUTGOING, directionEvidencePacket)
	require.Equal(t, network.OUTGOING, source.conn.Direction)
	require.Equal(t, directionEvidencePacket, source.directionEvidence)
}

func TestNStatTracerSkipsAmbiguousReversePeerDirection(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	for sourceRef, pid := range map[uint64]uint32{31: 4001, 32: 4002} {
		tracer.processEvent(nstat.Event{
			Kind:      nstat.EventDescription,
			SourceRef: sourceRef,
			Provider:  nstat.ProviderTCPKernel,
			Flow:      testNStatLoopbackTCPFlow(pid, "127.0.0.1", 8080, 50000),
		})
	}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 33,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatLoopbackTCPFlow(4003, "127.0.0.1", 50000, 8080),
	})
	tracer.setSourceDirection(tracer.sources[33], network.OUTGOING, directionEvidenceTCPState)
	tracer.reconcileSourceDirection(tracer.sources[33])

	require.Equal(t, network.OUTGOING, tracer.sources[33].conn.Direction)
	require.Equal(t, network.UNKNOWN, tracer.sources[31].conn.Direction)
	require.Equal(t, network.UNKNOWN, tracer.sources[32].conn.Direction)
}

func TestNStatTracerClosesFailedAttemptWithoutOverwritingFailure(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(200, 0)
	tracer.now = func() time.Time { return now }
	var closed *network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) { closed = conn }

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventUpdate,
		SourceRef: 8,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(2345, tcpStateSynSent),
		Counts: &nstat.Counts{
			ConnectAttempts: 1,
		},
	})
	// Simulate a refusal already attributed by the packet sidecar.
	tracer.sources[8].conn.TCPFailures = map[uint16]uint32{
		network.TCPFailureErrnoConnRefused: 1,
	}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventUpdate,
		SourceRef: 8,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(2345, tcpStateClosed),
		Counts:    &nstat.Counts{},
	})

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	failed := buffer.Connections()[0]
	require.Equal(t, map[uint16]uint32{network.TCPFailureErrnoConnRefused: 1}, failed.TCPFailures)
	require.Equal(t, network.OUTGOING, failed.Direction)
	require.Equal(t, uint16(1), failed.Monotonic.TCPClosed)
	require.True(t, failed.IsClosed)

	now = now.Add(time.Second)
	tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 8})

	require.NotNil(t, closed)
	require.Equal(t, uint16(1), closed.Monotonic.TCPClosed)
	require.Equal(t, map[uint16]uint32{network.TCPFailureErrnoConnRefused: 1}, closed.TCPFailures)
}

func TestNStatTracerDeliversLateDescriptionAfterRemoval(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	tracer.now = func() time.Time { return time.Unix(300, 0) }
	var closed *network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) { closed = conn }

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventAdded,
		SourceRef: 9,
		Provider:  nstat.ProviderUDPKernel,
	})
	tracer.pumpDescriptionRequests(tracer.now())
	tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 9})
	tracer.pumpDescriptionRequests(tracer.now())
	require.Equal(t, []uint64{9, 9}, control.descriptions())

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 9,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      testNStatUDPFlow(3456),
	})

	require.NotNil(t, closed)
	require.Equal(t, network.UDP, closed.Type)
	require.Equal(t, uint32(3456), closed.Pid)
	require.True(t, closed.IsClosed)
	require.Zero(t, closed.Monotonic.TCPClosed)
}

func TestNStatTracerSeparatesReusedSourceReference(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	tracer.now = func() time.Time { return time.Unix(350, 0) }
	var closed []*network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) {
		closed = append(closed, conn)
	}

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 10,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(1111, tcpStateSynSent),
		Counts:    &nstat.Counts{ConnectAttempts: 1, ConnectSuccesses: 1},
	})
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	firstCookie := buffer.Connections()[0].Cookie
	require.Equal(t, network.OUTGOING, buffer.Connections()[0].Direction)
	require.Equal(t, directionEvidenceTCPState, tracer.sources[10].directionEvidence)

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 10,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(2222, tcpStateEstablished),
	})

	require.Len(t, closed, 1)
	require.Equal(t, uint32(1111), closed[0].Pid)
	buffer.Reset()
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, uint32(2222), buffer.Connections()[0].Pid)
	require.NotEqual(t, firstCookie, buffer.Connections()[0].Cookie)
	require.Equal(t, network.UNKNOWN, buffer.Connections()[0].Direction)
	require.Zero(t, tracer.sources[10].connectAttempts)
	require.Equal(t, directionEvidenceNone, tracer.sources[10].directionEvidence)
}

func TestNStatTracerReindexesReusedAndRemovedListenerSource(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 11,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPListenerFlow(5001, "127.0.0.1", 8080),
	})
	require.True(t, tracer.sources[11].listenerIndexed)
	require.Len(t, tracer.listeners, 1)

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 11,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPListenerFlow(5002, "127.0.0.1", 8081),
	})
	require.Equal(t, uint32(5002), tracer.sources[11].flow.PID)
	require.Equal(t, uint16(8081), tracer.sources[11].listenerKey.port)
	require.Len(t, tracer.listeners, 1)

	tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 11})
	require.False(t, tracer.sources[11].listenerIndexed)
	require.Empty(t, tracer.listeners)
}

func TestNStatTracerWaitsForCompleteTCPTuple(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(360, 0)
	tracer.now = func() time.Time { return now }
	partial := testNStatTCPFlow(3333, tcpStateSynSent)
	partial.Local.Address = netip.IPv4Unspecified()
	partial.Remote.Address = netip.IPv4Unspecified()
	partial.Remote.Port = 0
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 14,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      partial,
	})

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Empty(t, buffer.Connections())
	tracer.queueUnresolvedDescriptions(now)
	tracer.pumpDescriptionRequests(now)
	require.Equal(t, []uint64{14}, control.descriptions())

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 14,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(3333, tcpStateEstablished),
	})
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, netip.MustParseAddr("192.0.2.10"), buffer.Connections()[0].Source.Addr)
	require.Equal(t, netip.MustParseAddr("198.51.100.20"), buffer.Connections()[0].Dest.Addr)
	require.Equal(t, uint16(1), buffer.Connections()[0].Monotonic.TCPEstablished)
}

func TestNStatTracerAppliesLateAuthoritativePID(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 15,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(0, tcpStateEstablished),
	})
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 15,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(4321, tcpStateEstablished),
	})

	resolved, ambiguous, reuseRejected := tracer.reconcileLibprocSnapshot(libproc.Snapshot{
		Observations: []libproc.Observation{testDarwinLibprocObservation(9876, 1)},
	})

	require.Zero(t, resolved)
	require.Zero(t, ambiguous)
	require.Zero(t, reuseRejected)
	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	require.Equal(t, uint32(4321), buffer.Connections()[0].Pid)
}

func TestNStatTracerAppliesLateAuthoritativeUDPRemote(t *testing.T) {
	tracer := newNStatTracerWithControl(testNStatConfig(), newFakeNStatControl())
	initial := testNStatUDPFlow(1234)
	initial.Remote = nstat.Endpoint{}
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 16,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      initial,
	})

	var buffer network.ConnectionBuffer
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	oldTuple := buffer.Connections()[0].ConnectionTuple
	require.False(t, oldTuple.Dest.Addr.IsValid())

	update := testNStatUDPFlow(1234)
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 16,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      update,
	})

	buffer.Reset()
	require.NoError(t, tracer.GetConnections(&buffer, nil))
	require.Len(t, buffer.Connections(), 1)
	conn := buffer.Connections()[0]
	require.Equal(t, update.Remote.Address, conn.Dest.Addr)
	require.Equal(t, update.Remote.Port, conn.DPort)
	require.False(t, tracer.tuples.match(oldTuple).matched)
	match := tracer.tuples.match(conn.ConnectionTuple)
	require.True(t, match.matched)
	require.Equal(t, conn.Cookie, match.cookie)
}

func TestNStatTracerRuntimeFailureClosesActiveGenerationOnce(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(375, 0)
	tracer.now = func() time.Time { return now }
	var closed []*network.ConnectionStats
	tracer.closeCallback = func(conn *network.ConnectionStats) {
		closed = append(closed, conn)
	}
	var failures []error
	tracer.setRuntimeFailureCallback(func(err error) {
		failures = append(failures, err)
	})

	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 11,
		Provider:  nstat.ProviderTCPKernel,
		Flow:      testNStatTCPFlow(1111, tcpStateEstablished),
	})
	tracer.processEvent(nstat.Event{
		Kind:      nstat.EventDescription,
		SourceRef: 12,
		Provider:  nstat.ProviderUDPKernel,
		Flow:      testNStatUDPFlow(2222),
	})

	now = now.Add(2 * time.Second)
	runtimeErr := errors.New("ABI changed")
	tracer.handleRuntimeFailure(runtimeErr)
	tracer.handleRuntimeFailure(errors.New("second failure"))

	require.Equal(t, []error{runtimeErr}, failures)
	require.Len(t, closed, 2)
	for _, conn := range closed {
		require.True(t, conn.IsClosed)
		require.Equal(t, 2*time.Second, conn.Duration)
	}
	require.Empty(t, tracer.sources)
	var buffer network.ConnectionBuffer
	require.ErrorIs(t, tracer.GetConnections(&buffer, nil), runtimeErr)
}

func TestNStatTracerQueryAllTreatsBackpressureAsTransient(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)

	control.queryErr = unix.EAGAIN
	require.NoError(t, tracer.queryAll())

	control.queryErr = io.ErrUnexpectedEOF
	require.ErrorIs(t, tracer.queryAll(), io.ErrUnexpectedEOF)
}

func TestNStatTracerBoundsAndRetriesDescriptionRequests(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(390, 0)
	tracer.now = func() time.Time { return now }
	for sourceRef := uint64(1); sourceRef <= nstatDescriptionBatch+2; sourceRef++ {
		tracer.processEvent(nstat.Event{
			Kind:      nstat.EventAdded,
			SourceRef: sourceRef,
			Provider:  nstat.ProviderTCPKernel,
		})
	}

	tracer.pumpDescriptionRequests(now)
	require.Len(t, control.descriptions(), nstatDescriptionBatch)
	tracer.queueUnresolvedDescriptions(now)
	tracer.pumpDescriptionRequests(now)
	require.Len(t, control.descriptions(), nstatDescriptionBatch+2)

	now = now.Add(nstatDescriptionRetry)
	tracer.queueUnresolvedDescriptions(now)
	tracer.pumpDescriptionRequests(now)
	require.Len(t, control.descriptions(), 2*nstatDescriptionBatch+2)
}

func TestNStatTracerPrioritizesNewDescriptionsWithoutStarvingRetries(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)
	now := time.Unix(395, 0)
	tracer.now = func() time.Time { return now }

	for sourceRef := uint64(1); sourceRef <= nstatDescriptionBatch+2; sourceRef++ {
		tracer.processEvent(nstat.Event{
			Kind:      nstat.EventDescription,
			SourceRef: sourceRef,
			Provider:  nstat.ProviderTCPKernel,
			Flow: &nstat.Flow{
				PID: uint32(sourceRef),
				Local: nstat.Endpoint{
					Address: netip.MustParseAddr("192.0.2.10"),
					Port:    uint16(40000 + sourceRef),
					Present: true,
				},
			},
		})
		tracer.mu.Lock()
		tracer.queueDescriptionLocked(sourceRef, tracer.sources[sourceRef])
		tracer.mu.Unlock()
	}

	for _, sourceRef := range []uint64{100, 101} {
		tracer.processEvent(nstat.Event{
			Kind:      nstat.EventAdded,
			SourceRef: sourceRef,
			Provider:  nstat.ProviderUDPKernel,
		})
	}

	tracer.pumpDescriptionRequests(now)
	require.Equal(t, []uint64{100, 101, 1, 2, 3, 4, 5, 6}, control.descriptions())

	tracer.pumpDescriptionRequests(now)
	require.Equal(t, []uint64{100, 101, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, control.descriptions())
}

func TestNStatTracerStartAndStop(t *testing.T) {
	control := newFakeNStatControl()
	tracer := newNStatTracerWithControl(testNStatConfig(), control)

	require.NoError(t, tracer.Start(func(*network.ConnectionStats) {}))
	require.ErrorContains(t, tracer.Start(func(*network.ConnectionStats) {}), "already started")
	<-control.pollStarted
	tracer.Stop()
	tracer.Stop()
	require.ErrorIs(t, tracer.Start(func(*network.ConnectionStats) {}), io.ErrClosedPipe)

	require.Equal(t, nstatProviders[:], control.subscriptions())
	require.True(t, control.isClosed())
}

func TestNStatTracerSubscribesOnlyToEnabledProtocols(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*config.Config)
		expected  []uint32
	}{
		{
			name: "tcp only",
			configure: func(cfg *config.Config) {
				cfg.CollectUDPv4Conns = false
				cfg.CollectUDPv6Conns = false
			},
			expected: []uint32{nstat.ProviderTCPKernel, nstat.ProviderTCPUserland},
		},
		{
			name: "udp only",
			configure: func(cfg *config.Config) {
				cfg.CollectTCPv4Conns = false
				cfg.CollectTCPv6Conns = false
			},
			expected: []uint32{nstat.ProviderUDPKernel, nstat.ProviderUDPUserland},
		},
		{
			name: "disabled",
			configure: func(cfg *config.Config) {
				cfg.CollectTCPv4Conns = false
				cfg.CollectTCPv6Conns = false
				cfg.CollectUDPv4Conns = false
				cfg.CollectUDPv6Conns = false
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testNStatConfig()
			test.configure(cfg)
			control := newFakeNStatControl()
			tracer := newNStatTracerWithControl(cfg, control)

			require.NoError(t, tracer.subscribe())
			require.Equal(t, test.expected, control.subscriptions())
		})
	}
}

func TestNStatTracerHonorsProtocolAndFamilyConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		provider  uint32
		flow      func() *nstat.Flow
		configure func(*config.Config)
	}{
		{
			name:     "tcp4 disabled",
			provider: nstat.ProviderTCPKernel,
			flow:     func() *nstat.Flow { return testNStatTCPFlow(1234, tcpStateEstablished) },
			configure: func(cfg *config.Config) {
				cfg.CollectTCPv4Conns = false
			},
		},
		{
			name:     "tcp6 disabled",
			provider: nstat.ProviderTCPKernel,
			flow:     func() *nstat.Flow { return testNStatTCPv6Flow(1234, tcpStateEstablished) },
			configure: func(cfg *config.Config) {
				cfg.CollectTCPv6Conns = false
			},
		},
		{
			name:     "udp4 disabled",
			provider: nstat.ProviderUDPKernel,
			flow:     func() *nstat.Flow { return testNStatUDPFlow(1234) },
			configure: func(cfg *config.Config) {
				cfg.CollectUDPv4Conns = false
			},
		},
		{
			name:     "udp6 disabled",
			provider: nstat.ProviderUDPKernel,
			flow:     func() *nstat.Flow { return testNStatUDPv6Flow(1234) },
			configure: func(cfg *config.Config) {
				cfg.CollectUDPv6Conns = false
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testNStatConfig()
			test.configure(cfg)
			tracer := newNStatTracerWithControl(cfg, newFakeNStatControl())
			var closed []*network.ConnectionStats
			tracer.closeCallback = func(conn *network.ConnectionStats) {
				closed = append(closed, conn)
			}

			tracer.processEvent(nstat.Event{
				Kind:      nstat.EventDescription,
				SourceRef: 1,
				Provider:  test.provider,
				Flow:      test.flow(),
			})

			var buffer network.ConnectionBuffer
			require.NoError(t, tracer.GetConnections(&buffer, nil))
			require.Empty(t, buffer.Connections())
			tracer.processEvent(nstat.Event{Kind: nstat.EventRemoved, SourceRef: 1})
			require.Empty(t, closed)
		})
	}
}

func TestNStatConversions(t *testing.T) {
	require.Equal(t, uint64(0), saturatingSubtract(10, 11))
	require.Equal(t, uint64(0), saturatingSubtract(10, 10))
	require.Equal(t, uint64(9), saturatingSubtract(10, 1))
	require.Equal(t, uint32(28281), scaledMicroseconds(905, nstatTCPRTTScale))

	current := testNStatTCPFlow(1234, tcpStateEstablished)
	update := &nstat.Flow{
		TCPState: tcpStateClosed,
		Local: nstat.Endpoint{
			Address: netip.IPv4Unspecified(),
			Port:    current.Local.Port,
			Present: true,
		},
		Remote: nstat.Endpoint{
			Address: netip.IPv4Unspecified(),
			Present: true,
		},
	}
	merged := mergeNStatFlow(current, update)
	require.Equal(t, current.PID, merged.PID)
	require.Equal(t, current.UniquePID, merged.UniquePID)
	require.Equal(t, current.Local, merged.Local)
	require.Equal(t, current.Remote, merged.Remote)
	require.Equal(t, uint32(tcpStateClosed), merged.TCPState)
}

func testNStatConfig() *config.Config {
	return &config.Config{
		CollectTCPv4Conns:     true,
		CollectTCPv6Conns:     true,
		CollectUDPv4Conns:     true,
		CollectUDPv6Conns:     true,
		MaxTrackedConnections: 100,
	}
}

func testNStatTCPFlow(pid, state uint32) *nstat.Flow {
	return &nstat.Flow{
		UniquePID:      uint64(pid) << 32,
		PID:            pid,
		InterfaceIndex: 14,
		TCPState:       state,
		Local: nstat.Endpoint{
			Address: netip.MustParseAddr("192.0.2.10"),
			Port:    50000,
			Present: true,
		},
		Remote: nstat.Endpoint{
			Address: netip.MustParseAddr("198.51.100.20"),
			Port:    443,
			Present: true,
		},
	}
}

func testNStatTCPv6Flow(pid, state uint32) *nstat.Flow {
	flow := testNStatTCPFlow(pid, state)
	flow.Local.Address = netip.MustParseAddr("2001:db8::10")
	flow.Remote.Address = netip.MustParseAddr("2001:db8::20")
	return flow
}

func testNStatLoopbackTCPFlow(pid uint32, address string, localPort, remotePort uint16) *nstat.Flow {
	flow := testNStatTCPFlow(pid, tcpStateEstablished)
	flow.Local.Address = netip.MustParseAddr(address)
	flow.Local.Port = localPort
	flow.Remote.Address = netip.MustParseAddr(address)
	flow.Remote.Port = remotePort
	return flow
}

func testNStatTCPListenerFlow(pid uint32, address string, localPort uint16) *nstat.Flow {
	flow := testNStatLoopbackTCPFlow(pid, address, localPort, 1)
	flow.TCPState = tcpStateListen
	flow.Remote = nstat.Endpoint{}
	return flow
}

func testNStatUDPFlow(pid uint32) *nstat.Flow {
	flow := testNStatTCPFlow(pid, 0)
	return flow
}

func testNStatUDPv6Flow(pid uint32) *nstat.Flow {
	return testNStatTCPv6Flow(pid, 0)
}

type fakeNStatControl struct {
	mu sync.Mutex

	subscribed          []uint32
	descriptionRequests []uint64
	closed              chan struct{}
	closeOnce           sync.Once
	pollStarted         chan struct{}
	pollOnce            sync.Once
	queryErr            error
}

func newFakeNStatControl() *fakeNStatControl {
	return &fakeNStatControl{
		closed:      make(chan struct{}),
		pollStarted: make(chan struct{}),
	}
}

func (f *fakeNStatControl) Subscribe(provider uint32) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subscribed = append(f.subscribed, provider)
	return 0, nil
}

func (f *fakeNStatControl) QueryAll() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queryErr
}

func (f *fakeNStatControl) RequestDescription(sourceRef uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.descriptionRequests = append(f.descriptionRequests, sourceRef)
	return nil
}

func (f *fakeNStatControl) Poll(time.Duration) (bool, error) {
	f.pollOnce.Do(func() { close(f.pollStarted) })
	<-f.closed
	return false, io.ErrClosedPipe
}

func (f *fakeNStatControl) Receive([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func (f *fakeNStatControl) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

func (f *fakeNStatControl) subscriptions() []uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint32(nil), f.subscribed...)
}

func (f *fakeNStatControl) descriptions() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint64(nil), f.descriptionRequests...)
}

func (f *fakeNStatControl) isClosed() bool {
	select {
	case <-f.closed:
		return true
	default:
		return false
	}
}
