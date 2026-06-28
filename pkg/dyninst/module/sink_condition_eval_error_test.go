// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// TestSinkConditionEvalErrorCounters verifies that the sink increments
// the per-runtime condition-evaluation error counters based on the
// Condition_eval_error byte in each event header. The BPF program
// fails open on these errors, so each affected event still arrives
// through HandleEvent; the counters give operators fleet-level
// visibility across snapshots that may be sampled or pruned away.
//
// This test only exercises counter accounting (decoder is stubbed).
// End-to-end coverage of the rendered evaluationErrors messages lives
// in pkg/dyninst/decode/testdata/notcapturedreason_e2e.md, which
// drives each condition-error class through a local agent and asserts
// on the emitted snapshot JSON.
func TestSinkConditionEvalErrorCounters(t *testing.T) {
	s, _ := newTestSink()

	// Submit a mix of events spanning the four condition_eval_error
	// states: 0 = pass, 1 = generic error, 2 = nil pointer deref,
	// 3 = any/all iteration cap exhausted. Each event is a
	// single-fragment standalone (no return paired), so the buffer
	// immediately produces a Ready and emit runs.
	send := func(condErr uint8) {
		hdr := output.EventHeader{
			Prog_id:                   1,
			Goid:                      1,
			Stack_byte_depth:          100,
			Probe_id:                  0,
			Event_pairing_expectation: uint8(output.EventPairingExpectationNone),
			Condition_eval_error:      condErr,
			Ktime_ns:                  uint64(condErr) + 1, // distinct keys
		}
		ev := buildTestEvent(&hdr, nil, nil)
		require.NoError(t, s.HandleEvent(dispatcher.MakeTestingMessage(ev)))
	}

	const (
		nPass       = 3
		nOther      = 2
		nNilDeref   = 4
		nIterCapExh = 5
	)
	for range nPass {
		send(0)
	}
	for range nOther {
		send(1)
	}
	for range nNilDeref {
		send(2)
	}
	for range nIterCapExh {
		send(3)
	}

	stats := s.runtime.stats.asStats()
	require.EqualValues(t, nOther, stats["condition_eval_error_other"],
		"generic condition-eval errors should accumulate")
	require.EqualValues(t, nNilDeref, stats["condition_eval_error_nil_deref"],
		"nil-deref condition-eval errors should accumulate")
	require.EqualValues(t, nIterCapExh, stats["condition_iteration_cap_exhausted"],
		"iteration-cap-exhausted condition-eval errors should accumulate")
	// The "pass" events must not touch any of the three counters.
	require.EqualValues(t, nOther+nNilDeref+nIterCapExh,
		stats["condition_eval_error_other"].(uint64)+
			stats["condition_eval_error_nil_deref"].(uint64)+
			stats["condition_iteration_cap_exhausted"].(uint64),
		"pass events (condition_eval_error=0) must not increment counters")
}
