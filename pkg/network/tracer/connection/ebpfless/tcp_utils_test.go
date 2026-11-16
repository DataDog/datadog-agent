// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build (linux && bpf) || darwin

package ebpfless

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSentSeqTracker(t *testing.T) {
	type segment struct {
		seq, nextSeq uint32
		advanced     bool
		overlap      uint32
		retransmit   bool
	}
	for _, tc := range []struct {
		name     string
		segments []segment
	}{
		{
			name: "new_data_then_retransmit",
			segments: []segment{
				{seq: 1, nextSeq: 101, advanced: true},
				{seq: 101, nextSeq: 201, advanced: true},
				{seq: 101, nextSeq: 201, retransmit: true},
			},
		},
		{
			name: "partial_overlap_advances",
			segments: []segment{
				{seq: 1, nextSeq: 101, advanced: true},
				{seq: 51, nextSeq: 151, advanced: true, overlap: 50},
			},
		},
		{
			name: "keepalive_is_not_a_retransmit",
			segments: []segment{
				{seq: 1, nextSeq: 101, advanced: true},
				{seq: 100, nextSeq: 100},
			},
		},
		{
			name: "wraparound",
			segments: []segment{
				{seq: 0xffffff00, nextSeq: 0xffffffff, advanced: true},
				{seq: 0xffffffff, nextSeq: 0x0000000f, advanced: true},
				{seq: 0xffffff00, nextSeq: 0xffffffff, retransmit: true},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tracker SentSeqTracker
			for i, s := range tc.segments {
				advanced, overlap, retransmit := tracker.Observe(s.seq, s.nextSeq)
				require.Equal(t, s.advanced, advanced, "segment %d advanced", i)
				require.Equal(t, s.overlap, overlap, "segment %d overlap", i)
				require.Equal(t, s.retransmit, retransmit, "segment %d retransmit", i)
			}
		})
	}
}
