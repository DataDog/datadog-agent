#ifndef __FRAMING_H__
#define __FRAMING_H__

#ifndef CGO
#include "ktypes.h"
#endif

// This file must be kept in sync with the ../output/framing.go file.
// If adding new structure, update ../output/framing_align_test.go to check that structure
// memory layout.

// The reason for a return call being omitted.
typedef enum event_pairing_expectation {
  EVENT_PAIRING_EXPECTATION_NONE = 0,
  EVENT_PAIRING_ENTRY_PAIRING_EXPECTED = 1,
  EVENT_PAIRING_RETURN_PAIRING_EXPECTED = 2,
  EVENT_PAIRING_EXPECTATION_CALL_COUNT_EXCEEDED = 3,
  EVENT_PAIRING_EXPECTATION_CALL_MAP_FULL = 4,
  EVENT_PAIRING_EXPECTATION_BUFFER_FULL = 5, // only used in userspace
  EVENT_PAIRING_EXPECTATION_NONE_INLINED = 6,
  EVENT_PAIRING_EXPECTATION_NONE_NO_BODY = 7,
  EVENT_PAIRING_EXPECTATION_CONDITION_FAILED = 8,
  // Stamped on the single synthetic event the recovery probe emits
  // when a goroutine recovers a panic that has unwound one or more
  // probed frames. The header's panic_lo_depth / panic_hi_depth bound
  // the unwound depth range; userspace fans the panic-value payload
  // out across every in-flight invocation on the goid whose
  // StackByteDepth is in (lo, hi] via NotePanicUnwoundRange.
  EVENT_PAIRING_RETURN_PANIC_UNWOUND = 9,
} event_pairing_expectation_t;

// The message header used for the event program.
typedef struct di_event_header {
  // The number of bytes of data items and messages to follow, including
  // the size of this header. Most of the other headers are exclusive of their
  // own size, but for the snapshot header, the size of the header is included.
  // Must be the first field in the structure, both ebpf and decoding components
  // assume it is.
  uint32_t data_byte_len;

  // The ID of the program that produced this event.
  uint32_t prog_id;

  // The Go ID of the goroutine that produced this event.
  uint64_t goid;

  // The byte depth of the call from the root of the stack (used to pair calls with
  // their correspond returns, particularly in the case of recursive calls, but
  // also when recovering from a panic). Note that this is the offset of the
  // frame base from the root of the stack; this lets it be robust to stack
  // growth and shrinking.
  uint32_t stack_byte_depth;

  // ProbeID is an interned ID for the probe that produced this event.
  // It can be used to pair events with their corresponding return events.
  uint32_t probe_id;

  // The number of bytes for a stack trace that follows this header.
  uint16_t stack_byte_len;
  // Event pairing expectation marks whether a return event is expected for this
  // event and if not, why not.
  unsigned char event_pairing_expectation;
  // Set to non-zero when the condition expression could not be fully evaluated
  // (e.g. nil pointer in the dereference chain). The event is still emitted
  // (condition error treated as pass) but userspace should report the error.
  unsigned char condition_eval_error;

  // Continuation support for events that exceed the 32KiB scratch buffer.
  // When an event is split across multiple ringbuf submissions:
  //   seq=0, flags=0: single-fragment event (legacy, backward compatible)
  //   seq=0, flags&1: first fragment, more to follow
  //   seq>0, flags&1: middle fragment
  //   seq>0, flags=0: final fragment
  // All fragments share the same (goid, stack_byte_depth, probe_id, ktime_ns).
  uint16_t continuation_seq;
  // Bit 0: more fragments follow (1 = not final).
  unsigned char continuation_flags;
  char __padding[1];

  // Hash of the stack trace that follows this header.
  uint64_t stack_hash;

  // The timestamp of the event according to CLOCK_MONOTONIC.
  uint64_t ktime_ns;

  // Invocation ID: for entry events, equals ktime_ns (the entry probe's own
  // start_ns). For return events, equals the entry probe's ktime_ns (pulled
  // from in_progress_calls at return time). Continuation fragments inherit
  // the value from fragment 0. Userspace uses this to correlate entry and
  // return for a single invocation, and to disambiguate rapid sequential
  // calls with the same (goid, stack_byte_depth, probe_id).
  uint64_t entry_ktime_ns;

  // Recovery-probe-only fields. Zero on every non-recovery event (the
  // scratch buffer is zero-initialised by events_scratch_buf_init).
  //
  // panic_lo_depth and panic_hi_depth describe the stack-byte-depth
  // range of in-progress probed calls being unwound by panic-recover,
  // computed by SM_OP_PANIC_UNWIND_PREPARE as
  //   lo = stack_hi - p.sp        (exclusive lower bound)
  //   hi = stack_hi - p.startSP   (inclusive upper bound)
  // Userspace scans buffered entries with matching goid and
  // stack_byte_depth in (lo, hi] to finalize them as panic-unwound.
  uint32_t panic_lo_depth;
  uint32_t panic_hi_depth;
}
// Use aligned attribute to ensure that the size of the structure is a multiple
// of 8 bytes; the attribute leads to the compiler adding padding.
__attribute((aligned(8))) di_event_header_t;

// The maximum number of pcs in a captured stack trace.
#define STACK_DEPTH 511

// The pcs of a captured stack trace.
typedef struct stack_pcs {
  // The number of values in the pcs array.
  uint64_t len;
  // The pcs of the captured stack trace.
  uint64_t pcs[STACK_DEPTH];
} stack_pcs_t;

// The header of a data item.
typedef struct di_data_item_header {
  // The type of the data item.
  uint32_t type;
  // The length of the data item.
  uint32_t length;
  // The address of the data item in the user process's address space.
  uint64_t address;
} __attribute__((aligned(8))) di_data_item_header_t;

// Reasons a drop notification is sent on the side channel. Describe what
// userspace state a drop affected, not which BPF failure site caused it.
//
//  RETURN_LOST        — return-side submit failed with no fragments sent;
//                       the matching entry sits in userspace's pairing
//                       store. Userspace should emit the entry alone.
//  PARTIAL_ENTRY      — entry submit succeeded for fragments [0..last_seq],
//                       then subsequently failed. Userspace has or will
//                       receive exactly last_seq+1 entry fragments; treat
//                       them as a truncated complete entry.
//  PARTIAL_RETURN     — same as PARTIAL_ENTRY, but for the return side.
//  PANIC_UNWOUND_LOST — the runtime.recovery synthetic event for the
//                       range (panic_lo_depth, panic_hi_depth] on goid
//                       failed to submit. BPF already evicted the matching
//                       in_progress_calls slots; userspace should range-
//                       scan its buffer and emit every matching invocation
//                       as a truncated panic-unwound capture. probe_id,
//                       stack_byte_depth, last_seq and entry_ktime_ns are
//                       not meaningful for this reason.
typedef enum drop_reason {
  DROP_REASON_RETURN_LOST        = 1,
  DROP_REASON_PARTIAL_ENTRY      = 2,
  DROP_REASON_PARTIAL_RETURN     = 3,
  DROP_REASON_PANIC_UNWOUND_LOST = 4,
} drop_reason_t;

// Side-channel message published to drop_notify_ringbuf to inform userspace
// that a drop has affected buffered state for one invocation.
//
// Fixed 40-byte layout, kept in sync with ../output/framing_linux.go.
typedef struct di_drop_notification {
  uint32_t prog_id;
  uint32_t probe_id;
  uint64_t goid;
  uint32_t stack_byte_depth;
  // drop_reason_t value; stored as uint8_t for compactness.
  uint8_t drop_reason;
  uint8_t __padding[1];
  // continuation_seq of the last successfully submitted fragment. Ignored
  // when drop_reason == DROP_REASON_RETURN_LOST (no fragments exist) or
  // DROP_REASON_PANIC_UNWOUND_LOST (range applies to many invocations).
  uint16_t last_seq;
  // Invocation ID. For return-side drops, the entry's start_ns (pulled from
  // in_progress_calls). For entry-side drops, the entry's own start_ns. This
  // matches the entry_ktime_ns field on the main-channel event header so
  // userspace can correlate notifications with fragments by key. Ignored
  // when drop_reason == DROP_REASON_PANIC_UNWOUND_LOST.
  uint64_t entry_ktime_ns;
  // Unwound stack depth range (panic_lo_depth, panic_hi_depth]. Only
  // meaningful when drop_reason == DROP_REASON_PANIC_UNWOUND_LOST; zero
  // for all other reasons.
  uint32_t panic_lo_depth;
  uint32_t panic_hi_depth;
} __attribute__((aligned(8))) di_drop_notification_t;

#endif // __FRAMING_H__
