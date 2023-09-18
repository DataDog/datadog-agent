// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package offsetguess

import (
	"unsafe"

	"golang.org/x/exp/slices"
)

type offsetRange struct {
	Offset uint64
	Size   uint64
}

// skipOverlaps returns the next valid offset and a boolean indicating if there was any overlap detected.
func skipOverlaps(offset uint64, ranges []offsetRange) (uint64, bool) {
	overlapped := false
StartOver:
	for {
		for _, r := range ranges {
			next := r.Offset + r.Size
			if r.Offset <= offset && offset < next {
				offset = next
				overlapped = true
				// completely redo the inner loop
				// you must have no overlaps to return from the function
				continue StartOver
			}
		}
		break
	}
	return offset, overlapped
}

// the following range functions use the index of the current `GuessWhat` value,
// to select only the ranges of fields guessed beforehand, offset against the same base subject.

func (t *tracerOffsetGuesser) sockRanges() []offsetRange {
	idx := slices.Index([]GuessWhat{
		GuessSAddr,
		GuessDAddr,
		GuessDPort,
		GuessFamily,
		GuessSPort,
		GuessNetNS,
		GuessRTT,
		GuessRTT, // stand in for rtt_var, will never be matched
		GuessDAddrIPv6,
	}, GuessWhat(t.status.What))

	return []offsetRange{
		{t.status.Offset_saddr, uint64(unsafe.Sizeof(t.status.Saddr))},
		{t.status.Offset_daddr, uint64(unsafe.Sizeof(t.status.Daddr))},
		{t.status.Offset_dport, uint64(unsafe.Sizeof(t.status.Dport))},
		{t.status.Offset_family, uint64(unsafe.Sizeof(t.status.Family))},
		{t.status.Offset_sport, uint64(unsafe.Sizeof(t.status.Sport))},
		{t.status.Offset_netns, uint64(unsafe.Sizeof(t.status.Netns))},
		{t.status.Offset_rtt, uint64(unsafe.Sizeof(t.status.Rtt))},
		{t.status.Offset_rtt_var, uint64(unsafe.Sizeof(t.status.Rtt_var))},
		{t.status.Offset_daddr_ipv6, uint64(unsafe.Sizeof(t.status.Daddr_ipv6))},
	}[:idx]
}

func (t *tracerOffsetGuesser) flowI4Ranges() []offsetRange {
	idx := slices.Index([]GuessWhat{
		GuessSAddrFl4,
		GuessDAddrFl4,
		GuessSPortFl4,
		GuessDPortFl4,
	}, GuessWhat(t.status.What))

	return []offsetRange{
		{t.status.Offset_saddr_fl4, uint64(unsafe.Sizeof(t.status.Saddr_fl4))},
		{t.status.Offset_daddr_fl4, uint64(unsafe.Sizeof(t.status.Daddr_fl4))},
		{t.status.Offset_sport_fl4, uint64(unsafe.Sizeof(t.status.Sport_fl4))},
		{t.status.Offset_dport_fl4, uint64(unsafe.Sizeof(t.status.Dport_fl4))},
	}[:idx]
}

func (t *tracerOffsetGuesser) flowI6Ranges() []offsetRange {
	idx := slices.Index([]GuessWhat{
		GuessSAddrFl6,
		GuessDAddrFl6,
		GuessSPortFl6,
		GuessDPortFl6,
	}, GuessWhat(t.status.What))

	return []offsetRange{
		{t.status.Offset_saddr_fl6, uint64(unsafe.Sizeof(t.status.Saddr_fl6))},
		{t.status.Offset_daddr_fl6, uint64(unsafe.Sizeof(t.status.Daddr_fl6))},
		{t.status.Offset_sport_fl6, uint64(unsafe.Sizeof(t.status.Sport_fl6))},
		{t.status.Offset_dport_fl6, uint64(unsafe.Sizeof(t.status.Dport_fl6))},
	}[:idx]
}

func (t *tracerOffsetGuesser) skBuffRanges() []offsetRange {
	idx := slices.Index([]GuessWhat{
		GuessSKBuffSock,
		GuessSKBuffTransportHeader,
		GuessSKBuffHead,
	}, GuessWhat(t.status.What))

	return []offsetRange{
		{t.status.Offset_sk_buff_sock, sizeofSKBuffSock},
		{t.status.Offset_sk_buff_transport_header, sizeofSKBuffTransportHeader},
		{t.status.Offset_sk_buff_head, sizeofSKBuffHead},
	}[:idx]
}

func (c *conntrackOffsetGuesser) nfConnRanges() []offsetRange {
	idx := slices.Index([]GuessWhat{
		GuessCtTupleOrigin,
		GuessCtTupleReply,
		GuessCtStatus,
		GuessCtNet,
	}, GuessWhat(c.status.What))

	return []offsetRange{
		{c.status.Offset_origin, sizeofNfConntrackTuple},
		{c.status.Offset_reply, sizeofNfConntrackTuple},
		{c.status.Offset_status, uint64(unsafe.Sizeof(c.status.Status))},
		{c.status.Offset_netns, uint64(unsafe.Sizeof(c.status.Netns))},
	}[:idx]
}
