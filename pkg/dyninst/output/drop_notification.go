// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package output

// DropReason classifies *why* a drop happened at the BPF emission site.
// Pair with DropSide (which side of the entry/return pair was affected)
// and Last_seq (how many fragments reached userspace) to derive the
// full picture. Kept in sync with drop_reason_t in ../ebpf/framing.h.
//
// Invariants:
//   - FirstFlushFailed implies no fragments reached userspace; Last_seq
//     is unused.
//   - FragmentLimit and RingBufferFull are partial-only; at least
//     Last_seq+1 fragments reached userspace before the failure.
//   - Ringbuf rejection of the *first* flush maps to FirstFlushFailed,
//     not RingBufferFull.
type DropReason uint8

const (
	// DropReasonFirstFlushFailed: no fragments reached userspace. Used
	// for both first-flush scratch failures and ringbuf rejection on
	// fragment 0. Last_seq is unused.
	DropReasonFirstFlushFailed DropReason = 1

	// DropReasonFragmentLimit: the SM hit MAX_CONTINUATION_FRAGMENTS.
	// Always partial: fragments [0..Last_seq] reached userspace before
	// the cap fired.
	DropReasonFragmentLimit DropReason = 2

	// DropReasonRingBufferFull: bpf_ringbuf_output rejected a fragment
	// mid-stream. Always partial: fragments [0..Last_seq] reached
	// userspace.
	DropReasonRingBufferFull DropReason = 3

	// DropReasonPanicUnwoundLost: the runtime.recovery synthetic event for
	// the unwound range (Panic_lo_depth, Panic_hi_depth] on Goid failed to
	// submit. BPF has already evicted the matching in_progress_calls slots,
	// so userspace must range-scan its own pairing store and emit every
	// matching invocation as a truncated panic-unwound capture. Probe_id,
	// Stack_byte_depth, Last_seq and Entry_ktime_ns are not meaningful for
	// this reason.
	DropReasonPanicUnwoundLost DropReason = 4
)

// DropSide indicates which side of the entry/return pair a drop affected.
// Kept in sync with drop_side_t in ../ebpf/framing.h.
type DropSide uint8

const (
	// DropSideUnset is the zero value. eBPF always populates Side on
	// emission, so observing DropSideUnset indicates either a bug or a
	// notification produced by an older BPF build.
	DropSideUnset  DropSide = 0
	DropSideEntry  DropSide = 1
	DropSideReturn DropSide = 2
)
