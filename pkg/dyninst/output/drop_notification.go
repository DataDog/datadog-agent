// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package output

// DropReason classifies the effect a ring-buffer drop had on userspace
// buffered state. Values must be kept in sync with the drop_reason enum in
// ../ebpf/framing.h.
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
)
