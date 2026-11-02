#ifndef __CHASED_SLICES_H__
#define __CHASED_SLICES_H__

// Naive implementation of the visited set for strings and slices, using a
// sequential array.

#include "bpf_tracing.h"

typedef struct chased_slice {
  uint64_t addr;
  uint32_t type_id;
  uint32_t len;
} chased_slice_t;

#define MAX_CHASED_SLICES 128
typedef struct chased_slices {
  uint16_t len;
  chased_slice_t slices[MAX_CHASED_SLICES];
} chased_slices_t;

void chased_slices_init(chased_slices_t* slices) {
  slices->len = 0;
}

// chased_slices_push records that the data of a string or slice, len bytes
// of type type_id starting at addr, is about to be captured. It returns false,
// telling the caller not to capture the data, if an entry with the same
// address and type already covers at least len bytes, or if the set is full
// (MAX_CHASED_SLICES entries). Otherwise it appends an entry and returns true.
// For aliases of a buffer that differ in length, this captures the longest
// one: a shorter alias after a longer one is skipped, while a longer alias
// after a shorter one is captured again, and the decoder renders both from
// the longer data item.
//
// It is a global (non-static, noinline) function so that the verifier checks
// it once, on its own, instead of at every call site and on every path that
// reaches one. The verifier cannot prune iterations of the search loop, so
// each check unrolls all of them; inlined into the stack machine's bpf_loop
// callback, this function accounted for about 40% of the instructions the
// verifier processed for the whole program.
//
// The loop scans forwards: as a global function, the verifier only knows
// slices as a pointer to sizeof(chased_slices_t) bytes, and the compiled
// backwards scan reads before that range as far as the verifier can tell.
// The scan order does not matter, since only the existence of a match does.
//
// TODO(andrei): Only aliases that start at the same address are recognized.
// A string or slice that starts inside a captured one (e.g. buf[5:10] after
// buf[:100]) is captured again. Recognizing it would take a containment check
// here, e.g. a search of the captured ranges by address, and a decoder that
// resolves a pointer into the middle of a captured data item; today the
// decoder looks data items up by exact (type, address).
__attribute__((noinline)) int chased_slices_push(chased_slices_t* slices, uint64_t addr, uint32_t type_id, uint32_t len) {
  if (!slices) {
    LOG(1, "chased_slices_push: null %lld %d %d\n", addr, type_id, len);
    return false;
  }
  uint32_t slices_len = slices->len;
  if (slices_len >= MAX_CHASED_SLICES) {
    LOG(3, "chased_slices_push: full %lld %d %d\n", addr, type_id, len);
    return false;
  }
  for (uint32_t i = 0; i < slices_len; i++) {
    uint32_t j = i;
    barrier_var(j); // Needed up to 6.5.
    if (j >= MAX_CHASED_SLICES) {
      break;
    }
    if (slices->slices[j].addr == addr && slices->slices[j].type_id == type_id && slices->slices[j].len >= len) {
      return false;
    }
  }
  barrier_var(slices_len); // Needed up to 6.2.
  if (slices_len >= MAX_CHASED_SLICES) {
    return false;
  }
  slices->slices[slices_len] = (chased_slice_t){
      .addr = addr,
      .type_id = type_id,
      .len = len,
  };
  slices->len++;
  return true;
}

#endif // __CHASED_SLICES_H__
