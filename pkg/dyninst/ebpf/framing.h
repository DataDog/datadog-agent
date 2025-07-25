#ifndef __FRAMING_H__
#define __FRAMING_H__

#ifndef CGO
#include "ktypes.h"
#endif

// This file must be kept in sync with the ../output/framing.go file.
// If adding new structure, update ../output/framing_align_test.go to check that structure
// memory layout.

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

  // The number of bytes for a stack trace that follows this header.
  uint16_t stack_byte_len;
  char __padding[6];
 
  // Hash of the stack trace that follows this header.
  uint64_t stack_hash;

  // The timestamp of the event according to CLOCK_MONOTONIC.
  uint64_t ktime_ns;
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

#endif // __FRAMING_H__
