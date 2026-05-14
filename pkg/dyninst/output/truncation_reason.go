// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package output

// TruncationReason classifies *why* one side of a captured event
// (entry or return) is incomplete. It is the per-side counterpart to
// DataItemReason (which describes *why* an individual value is
// incomplete).
//
// TruncationReason is observed by userspace in two situations:
//   - Some fragments of a side reached userspace but the rest were
//     lost. Carried from eBPF via the drop notification side channel
//     (FragmentLimit -> EventTooLarge; RingBufferFull -> AgentOverloaded).
//   - The eventbuf detected loss without a structured notification
//     (drop-notify ringbuf overflowed and grace eviction fires); the
//     sink stamps UnknownLoss on the affected side.
//
// Whole-side losses (the side reached userspace as zero fragments)
// flow through the snapshot's evaluationErrors array with synthetic
// expression names "@return"/"@entry"; they do not use this enum.
type TruncationReason uint8

const (
	// TruncationReasonNone is the zero value: the side is complete.
	TruncationReasonNone TruncationReason = 0

	// TruncationReasonEventTooLarge: the event hit the per-event
	// fragment budget (MAX_CONTINUATION_FRAGMENTS × SCRATCH_BUF_LEN,
	// ≈512 KiB). Later fragments were never emitted.
	TruncationReasonEventTooLarge TruncationReason = 1

	// TruncationReasonAgentOverloaded: the agent's output ring buffer
	// rejected a fragment mid-stream because it was full. Indicates
	// load (slow consumer or high event rate), not a configuration
	// problem.
	TruncationReasonAgentOverloaded TruncationReason = 2

	// TruncationReasonUnknownLoss: data was lost but the structured
	// drop notification itself was lost. Used by the grace-window
	// eviction path on partial sides where we know the side is
	// incomplete but can't say why.
	TruncationReasonUnknownLoss TruncationReason = 3

	// TruncationReasonCaptureNestingTooDeep: the SM's recursion stack
	// was exhausted at a site (SM_OP_CALL overflow) where no specific
	// data item or field address was available, so the reason is
	// stamped at the side level rather than on a placeholder data
	// item.
	TruncationReasonCaptureNestingTooDeep TruncationReason = 4
)
