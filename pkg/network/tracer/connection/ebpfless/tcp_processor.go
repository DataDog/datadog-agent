// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package ebpfless

import (
	"fmt"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/google/gopacket/layers"
	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type connectionState struct {
	tcpState connStatus

	// hasSentPacket is whether anything has been sent outgoing (aka whether maxSeqSent exists)
	hasSentPacket bool
	// maxSeqSent is the latest outgoing tcp.Seq if hasSentPacket==true
	maxSeqSent uint32

	// hasLocalAck is whether there have been outgoing ACK's
	hasLocalAck bool
	// lastLocalAck is the latest outgoing tcp.Ack if hasLocalAck
	lastLocalAck uint32
	// hasRemoteAck is whether there have been incoming ACK's
	hasRemoteAck bool
	// lastRemoteAck is the latest incoming tcp.Ack if hasRemoteAck
	lastRemoteAck uint32

	// localSynState is the status of the outgoing SYN handshake
	localSynState synState
	// remoteSynState is the status of the incoming SYN handshake
	remoteSynState synState

	// hasLocalFin is whether the outgoing side has FIN'd
	hasLocalFin bool
	// hasRemoteFin is whether the incoming side has FIN'd
	hasRemoteFin bool
	// localFinSeq is the tcp.Seq number for the outgoing FIN (including any payload length)
	localFinSeq uint32
	// remoteFinSeq is the tcp.Seq number for the incoming FIN (including any payload length)
	remoteFinSeq uint32

	// rttTracker is used to track round trip times
	rttTracker rttTracker

	// lastUpdateEpoch contains the last timestamp this connection sent/received a packet
	// TODO find a way to combine this with ConnectionStats.lastUpdateEpoch
	// This exists because connections in pendingConns don't have a ConnectionStats object yet.
	// Can we make all connections in TCPProcessor have a ConnectionStats no matter what, and
	// filter them out in GetConnections?
	lastUpdateEpoch uint64

	// connDirection has the direction of the connection, if we saw the SYN packet
	connDirection network.ConnectionDirection
}

func (st *connectionState) hasMissedHandshake() bool {
	return st.localSynState == synStateMissed || st.remoteSynState == synStateMissed
}

// TCPProcessor encapsulates TCP state tracking for the ebpfless tracer
type TCPProcessor struct {
	cfg *config.Config
	// pendingConns contains connections with tcpState == connStatAttempted
	pendingConns map[PCAPTuple]*connectionState
	// establishedConns contains connections with tcpState == connStatEstablished
	establishedConns map[PCAPTuple]*connectionState
	// recentlyClosed is analogous to the TIME_WAIT state after a connection closes. we use this to
	// suppress "pre-existing connection" logic on retransmits after a connection closes
	recentlyClosed *expirable.LRU[PCAPTuple, struct{}]
}

// TODO make this into a config value
const maxPendingConns = 4096
const pendingConnTimeoutNs = uint64(5 * time.Second)

// In practice, linux remembers connections for 1 minute after close (see TCP_TIMEWAIT_LEN).
// I don't think the exact value matters though, since we use an LRU cache regardless
const recentlyClosedTimeout = 1 * time.Minute

// NewTCPProcessor constructs an empty TCPProcessor
func NewTCPProcessor(cfg *config.Config) *TCPProcessor {
	return &TCPProcessor{
		cfg:              cfg,
		pendingConns:     make(map[PCAPTuple]*connectionState, maxPendingConns),
		establishedConns: make(map[PCAPTuple]*connectionState, cfg.MaxTrackedConnections),
		recentlyClosed:   expirable.NewLRU[PCAPTuple, struct{}](int(cfg.MaxTrackedConnections), nil, recentlyClosedTimeout),
	}
}

// updateConnStatsForOpen sets the duration to a "timestamp" representing the open time
func updateConnStatsForOpen(conn *network.ConnectionStats) {
	conn.IsClosed = false
	conn.Duration = time.Duration(time.Now().UnixNano())
}

// updateConnStatsForClose writes the actual duration once the connection closed
func updateConnStatsForClose(conn *network.ConnectionStats) {
	conn.IsClosed = true
	nowNs := time.Now().UnixNano()
	conn.Duration = time.Duration(nowNs - int64(conn.Duration))
}

// calcNextSeq returns the seq "after" this segment, aka, what the ACK will be once this segment is received
func calcNextSeq(tcp *layers.TCP, payloadLen uint16) uint32 {
	nextSeq := tcp.Seq + uint32(payloadLen)
	if tcp.SYN || tcp.FIN {
		nextSeq++
	}
	return nextSeq
}

func checkInvalidTCP(tcp *layers.TCP) bool {
	noFlagsCombo := !tcp.SYN && !tcp.FIN && !tcp.ACK && !tcp.RST
	if noFlagsCombo {
		// no flags at all (I think this can happen for expanding the TCP window sometimes?)
		statsTelemetry.missingTCPFlags.Inc()
		return true
	} else if tcp.SYN && tcp.FIN {
		statsTelemetry.tcpSynAndFin.Inc()
		return true
	} else if tcp.RST && tcp.SYN {
		statsTelemetry.tcpRstAndSyn.Inc()
		return true
	} else if tcp.RST && tcp.FIN {
		statsTelemetry.tcpRstAndFin.Inc()
		return true
	}

	return false
}

func (t *TCPProcessor) updateSynFlag(conn *network.ConnectionStats, st *connectionState, pktType uint8, tcp *layers.TCP, _payloadLen uint16) {
	if tcp.RST {
		return
	}
	// if this is the initial SYN, store the connection direction
	if tcp.SYN && !tcp.ACK {
		st.connDirection = connDirectionFromPktType(pktType)
	}
	// progress the synStates based off this packet
	if pktType == unix.PACKET_OUTGOING {
		st.localSynState.update(tcp.SYN, tcp.ACK)
	} else {
		st.remoteSynState.update(tcp.SYN, tcp.ACK)
	}
	// if any synState has progressed, move to attempted
	if st.tcpState == connStatClosed && (st.localSynState != synStateNone || st.remoteSynState != synStateNone) {
		st.tcpState = connStatAttempted

		updateConnStatsForOpen(conn)
	}
	// if both synStates are ack'd, move to established
	if st.tcpState == connStatAttempted && st.localSynState.isSynAcked() && st.remoteSynState.isSynAcked() {
		st.tcpState = connStatEstablished
		if st.hasMissedHandshake() {
			statsTelemetry.missedTCPHandshakes.Inc()
		} else {
			conn.Monotonic.TCPEstablished++
		}
	}
}

// updateTCPStats is designed to mirror the stat tracking in the windows driver's handleFlowProtocolTcp
// https://github.com/DataDog/datadog-windows-filter/blob/d7560d83eb627117521d631a4c05cd654a01987e/ddfilter/flow/flow_tcp.c#L91
func (t *TCPProcessor) updateTCPStats(conn *network.ConnectionStats, st *connectionState, pktType uint8, tcp *layers.TCP, payloadLen uint16, timestampNs uint64) {
	nextSeq := calcNextSeq(tcp, payloadLen)

	st.lastUpdateEpoch = timestampNs
	if pktType == unix.PACKET_OUTGOING {
		conn.Monotonic.SentPackets++
		// packetCanRetransmit filters out packets that look like retransmits but aren't, like TCP keepalives
		packetCanRetransmit := nextSeq != tcp.Seq
		if !st.hasSentPacket || isSeqBefore(st.maxSeqSent, nextSeq) {
			st.hasSentPacket = true
			conn.Monotonic.SentBytes += uint64(payloadLen)
			st.maxSeqSent = nextSeq

			st.rttTracker.processOutgoing(timestampNs, nextSeq)
		} else if packetCanRetransmit {
			conn.Monotonic.Retransmits++

			st.rttTracker.clearTrip()
		}

		ackOutdated := !st.hasLocalAck || isSeqBefore(st.lastLocalAck, tcp.Ack)
		if tcp.ACK && ackOutdated {
			// wait until data comes in via synStateAcked
			if st.hasLocalAck && st.remoteSynState.isSynAcked() {
				ackDiff := tcp.Ack - st.lastLocalAck
				isFinAck := st.hasRemoteFin && tcp.Ack == st.remoteFinSeq
				if isFinAck {
					// if this is ack'ing a fin packet, there is an extra sequence number to cancel out
					ackDiff--
				}
				conn.Monotonic.RecvBytes += uint64(ackDiff)
			}

			st.hasLocalAck = true
			st.lastLocalAck = tcp.Ack
		}
	} else {
		conn.Monotonic.RecvPackets++

		ackOutdated := !st.hasRemoteAck || isSeqBefore(st.lastRemoteAck, tcp.Ack)
		if tcp.ACK && ackOutdated {
			st.hasRemoteAck = true
			st.lastRemoteAck = tcp.Ack

			hasNewRoundTrip := st.rttTracker.processIncoming(timestampNs, tcp.Ack)
			if hasNewRoundTrip {
				conn.RTT = nanosToMicros(st.rttTracker.rttSmoothNs)
				conn.RTTVar = nanosToMicros(st.rttTracker.rttVarNs)
			}
		}
	}
}

func (t *TCPProcessor) updateFinFlag(conn *network.ConnectionStats, st *connectionState, pktType uint8, tcp *layers.TCP, payloadLen uint16) {
	nextSeq := calcNextSeq(tcp, payloadLen)
	// update FIN sequence numbers
	if tcp.FIN {
		if pktType == unix.PACKET_OUTGOING {
			st.hasLocalFin = true
			st.localFinSeq = nextSeq
		} else {
			st.hasRemoteFin = true
			st.remoteFinSeq = nextSeq
		}
	}

	// if both fins have been sent and ack'd, then mark the connection closed
	localFinIsAcked := st.hasLocalFin && isSeqBeforeEq(st.localFinSeq, st.lastRemoteAck)
	remoteFinIsAcked := st.hasRemoteFin && isSeqBeforeEq(st.remoteFinSeq, st.lastLocalAck)
	if st.tcpState == connStatEstablished && localFinIsAcked && remoteFinIsAcked {
		*st = connectionState{
			tcpState: connStatClosed,
		}
		conn.Monotonic.TCPClosed++
		updateConnStatsForClose(conn)
	}
}

func (t *TCPProcessor) updateRstFlag(conn *network.ConnectionStats, st *connectionState, _pktType uint8, tcp *layers.TCP, _payloadLen uint16) {
	if !tcp.RST || st.tcpState == connStatClosed {
		return
	}

	reason := syscall.ECONNRESET
	if st.tcpState == connStatAttempted {
		reason = syscall.ECONNREFUSED
	}

	if conn.TCPFailures == nil {
		conn.TCPFailures = make(map[uint16]uint32)
	}
	conn.TCPFailures[uint16(reason)]++

	if st.tcpState != connStatClosed {
		conn.Monotonic.TCPClosed++
	}
	*st = connectionState{
		tcpState: connStatClosed,
	}
	updateConnStatsForClose(conn)
}

// Process handles a TCP packet, calculating stats and keeping track of its state according to the
// TCP state machine.
func (t *TCPProcessor) Process(conn *network.ConnectionStats, timestampNs uint64, pktType uint8, ip4 *layers.IPv4, ip6 *layers.IPv6, tcp *layers.TCP) (ProcessResult, error) {
	if pktType != unix.PACKET_OUTGOING && pktType != unix.PACKET_HOST {
		return ProcessResultNone, fmt.Errorf("TCPProcessor saw invalid pktType: %d", pktType)
	}
	payloadLen, err := TCPPayloadLen(conn.Family, ip4, ip6, tcp)
	if err != nil {
		return ProcessResultNone, err
	}

	log.TraceFunc(func() string {
		return "tcp processor: " + debugPacketInfo(pktType, tcp, payloadLen)
	})

	// skip invalid packets we don't recognize:
	if checkInvalidTCP(tcp) {
		return ProcessResultNone, nil
	}

	tuple := MakeEbpflessTuple(conn.ConnectionTuple)
	st, ok := t.getConn(tuple)
	if !ok {
		// create a fresh state object that will be stored by moveConn later
		st = &connectionState{}
	}
	origState := st.tcpState

	if tcp.SYN {
		// for some reason they're trying to re-use a recently-closed port
		t.removeRecentlyClosed(tuple)
	} else if t.isRecentlyClosed(tuple) {
		// if it's recently closed, we don't want to run TCP state tracking on the retransmits.
		// this avoids turning retransmits on recently closed connections into "partial connections"
		return ProcessResultNone, nil
	}

	t.updateSynFlag(conn, st, pktType, tcp, payloadLen)
	t.updateTCPStats(conn, st, pktType, tcp, payloadLen, timestampNs)
	t.updateFinFlag(conn, st, pktType, tcp, payloadLen)
	t.updateRstFlag(conn, st, pktType, tcp, payloadLen)

	// sync the ConnectionStats direction if necessary
	if conn.Direction == network.UNKNOWN {
		conn.Direction = st.connDirection
	}

	stateChanged := st.tcpState != origState
	if stateChanged {
		ok := t.moveConn(tuple, st)
		// if the map is full then we are unable to move the connection, report that
		if !ok {
			return ProcessResultMapFull, nil
		}
	}

	// if the connection is still established, we should update the connection map
	if st.tcpState == connStatEstablished {
		return ProcessResultStoreConn, nil
	}
	// if the connection just closed, store it in the tracer's closeCallback
	if st.tcpState == connStatClosed && stateChanged {
		t.markRecentlyClosed(tuple)
		return ProcessResultCloseConn, nil
	}
	return ProcessResultNone, nil
}

func (t *TCPProcessor) getConn(tuple PCAPTuple) (*connectionState, bool) {
	if st, ok := t.establishedConns[tuple]; ok {
		return st, true
	}
	if st, ok := t.pendingConns[tuple]; ok {
		return st, true
	}
	return nil, false
}

// RemoveConn clears a ConnectionTuple from its internal state.
func (t *TCPProcessor) RemoveConn(tuple PCAPTuple) {
	delete(t.pendingConns, tuple)
	delete(t.establishedConns, tuple)
}

var pendingConnFullLimiter = log.NewLogLimit(20, 10*time.Minute)
var establishedConnFullLimiter = log.NewLogLimit(20, 10*time.Minute)

// moveConn moves a connection to the correct map based on its tcpState.
// If it had to drop the connection because the target map was full, it returns false.
func (t *TCPProcessor) moveConn(tuple PCAPTuple, st *connectionState) bool {
	t.RemoveConn(tuple)

	switch st.tcpState {
	// For this case, simply let closed connections disappear. Process() will return
	// ProcessResultCloseConn letting the ebpfless tracer know the connection has closed.
	case connStatClosed:
	case connStatAttempted:
		ok := WriteMapWithSizeLimit(t.pendingConns, tuple, st, maxPendingConns)
		if !ok {
			statsTelemetry.droppedPendingConns.Inc()
			if pendingConnFullLimiter.ShouldLog() {
				log.Warnf("pending connections buffer filled to %d, connection droped", maxPendingConns)
			}
		}
		return ok
	case connStatEstablished:
		maxTrackedConns := int(t.cfg.MaxTrackedConnections)
		ok := WriteMapWithSizeLimit(t.establishedConns, tuple, st, maxTrackedConns)
		if !ok {
			statsTelemetry.droppedEstablishedConns.Inc()
			if establishedConnFullLimiter.ShouldLog() {
				log.Warnf("established connections buffer filled to %d, connection droped", maxTrackedConns)
			}
		}
		return ok
	}
	return true
}

// CleanupExpiredPendingConns iterates through pendingConns and removes those that
// have existed too long - in normal TCP, they should become established right away.
//
// This is only required for pendingConns because the tracer already has logic to remove
// established connections (connections that have ConnectionStats)
func (t *TCPProcessor) CleanupExpiredPendingConns(timestampNs uint64) {
	for tuple, st := range t.pendingConns {
		timeoutTime := st.lastUpdateEpoch + pendingConnTimeoutNs

		if timeoutTime <= timestampNs {
			delete(t.pendingConns, tuple)

			statsTelemetry.expiredPendingConns.Inc()
		}
	}
}

// MakeEbpflessTuple converts a network.ConnectionTuple to a PCAPTuple.
// See the PCAPTuple doc for more information.
func MakeEbpflessTuple(tuple network.ConnectionTuple) PCAPTuple {
	ret := PCAPTuple(tuple)
	ret.Pid = 0
	ret.Direction = network.UNKNOWN
	return ret
}

// MakeConnStatsTuple converts a PCAPTuple to a network.ConnectionTuple.
func MakeConnStatsTuple(tuple PCAPTuple) network.ConnectionTuple {
	// Direction is still 0, this will get set by the ebpfless tracer in finalizeConnectionDirection
	return network.ConnectionTuple(tuple)
}

func (t *TCPProcessor) markRecentlyClosed(tuple PCAPTuple) bool {
	return t.recentlyClosed.Add(tuple, struct{}{})
}

func (t *TCPProcessor) removeRecentlyClosed(tuple PCAPTuple) bool {
	return t.recentlyClosed.Remove(tuple)
}

func (t *TCPProcessor) isRecentlyClosed(tuple PCAPTuple) bool {
	_, ok := t.recentlyClosed.Peek(tuple)
	return ok
}
