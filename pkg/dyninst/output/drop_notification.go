// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package output

// DropReason classifies the effect a ring-buffer drop had on userspace
// buffered state. Values must be kept in sync with the drop_reason enum in
// ../ebpf/framing.h.
//
// A follow-up will replace these values with cause-explicit codes
// (FirstFlushFailed, FragmentLimit, RingBufferFull) and pair them with
// DropSide so userspace can disambiguate "the agent is overloaded" from
// "the operator asked for too much data per event".
type DropReason uint8

const (
	// DropReasonReturnLost: the return event (or its condition-failed /
	// throttle signal) could not be submitted, and no fragments of it were
	// ever sent. The pairing store holds a fully assembled entry. Userspace
	// should emit the entry alone.
	DropReasonReturnLost DropReason = 1

	// DropReasonPartialEntry: fragments [0..Last_seq] of the entry event
	// were submitted successfully; a subsequent submit failed. Userspace has
	// or will receive exactly Last_seq+1 entry fragments and should treat
	// them as a truncated complete entry.
	DropReasonPartialEntry DropReason = 2

	// DropReasonPartialReturn: same as DropReasonPartialEntry, but for the
	// return side.
	DropReasonPartialReturn DropReason = 3

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
//
// Until eBPF starts populating the Side field on the drop notification
// struct, callers will observe DropSideUnset (zero) and must fall back
// to inferring side from DropReason (RETURN_LOST/PARTIAL_RETURN → return,
// PARTIAL_ENTRY → entry). The follow-up that switches drop_reason_t to
// cause-explicit codes also starts populating Side at every emission
// site in event.c.
type DropSide uint8

const (
	// DropSideUnset is the zero value, observed when reading drop
	// notifications produced by eBPF that has not yet been updated to
	// populate Side.
	DropSideUnset  DropSide = 0
	DropSideEntry  DropSide = 1
	DropSideReturn DropSide = 2
)
