// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build (linux && linux_bpf) || darwin

package ebpfless

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func absDiff(a uint64, b uint64) uint64 {
	if a < b {
		return b - a
	}
	return a - b
}

// nanosToMicros converts nanoseconds to microseconds, rounding and converting to
// the uint32 type that ConnectionStats uses
func nanosToMicros(nanos uint64) uint32 {
	micros := time.Duration(nanos).Round(time.Microsecond).Microseconds()
	return uint32(micros)
}

// rttTracker implements the RTT algorithm specified here:
// https://datatracker.ietf.org/doc/html/rfc6298#section-2
type rttTracker struct {
	// sampleSentTimeNs is the timestamp our current round trip began.
	// If it is 0, there is nothing in flight or a retransmit cleared this
	sampleSentTimeNs uint64
	// expectedAck is the ack needed to complete the round trip
	expectedAck uint32
	// rttSmoothNs is the smoothed RTT in nanoseconds
	rttSmoothNs uint64
	// rttVarNs is the variance of the RTT in nanoseconds
	rttVarNs uint64
}

func (rt *rttTracker) isActive() bool {
	return rt.sampleSentTimeNs > 0
}

// processOutgoing is called to (potentially) start a round trip.
// Records the time of the packet for later
func (rt *rttTracker) processOutgoing(timestampNs uint64, nextSeq uint32) {
	if !rt.isActive() {
		rt.sampleSentTimeNs = timestampNs
		rt.expectedAck = nextSeq
	}
}

// clearTrip is called by a retransmit or when a round-trip completes
// Retransmits pollute RTT accuracy and cause a trip to be thrown out
func (rt *rttTracker) clearTrip() {
	if rt.isActive() {
		rt.sampleSentTimeNs = 0
		rt.expectedAck = 0
	}
}

// processIncoming is called to (potentially) close out a round trip.
// Based off this https://github.com/DataDog/datadog-windows-filter/blob/d7560d83eb627117521d631a4c05cd654a01987e/ddfilter/flow/flow_tcp.c#L269
// Returns whether the RTT stats were updated.
func (rt *rttTracker) processIncoming(timestampNs uint64, ack uint32) bool {
	hasCompletedTrip := rt.isActive() && isSeqBeforeEq(rt.expectedAck, ack)
	if !hasCompletedTrip {
		return false
	}

	elapsedNs := timestampNs - rt.sampleSentTimeNs
	if timestampNs < rt.sampleSentTimeNs {
		log.Warn("rttTracker encountered non-monotonic clock")
		elapsedNs = 0
	}
	rt.clearTrip()

	if rt.rttSmoothNs == 0 {
		rt.rttSmoothNs = elapsedNs
		rt.rttVarNs = elapsedNs / 2
		return true
	}

	// update variables based on fixed point math.
	// RFC 6298 says alpha=1/8 and beta=1/4
	const fixedBasis uint64 = 1000
	// SRTT < -(1 - alpha) * SRTT + alpha * R'
	oneMinusAlpha := fixedBasis - (fixedBasis / 8)
	alphaRPrime := elapsedNs / 8
	s := ((oneMinusAlpha * rt.rttSmoothNs) / fixedBasis) + alphaRPrime
	rt.rttSmoothNs = s

	// RTTVAR <- (1 - beta) * RTTVAR + beta * |SRTT - R'|
	oneMinusBeta := fixedBasis - fixedBasis/4
	rt.rttVarNs = (oneMinusBeta*rt.rttVarNs)/fixedBasis + absDiff(rt.rttSmoothNs, elapsedNs)/4

	return true
}
