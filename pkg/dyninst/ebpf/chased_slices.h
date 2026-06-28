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

// Result of chased_slices_push. INSERTED is "newly tracked, proceed";
// ALREADY_PRESENT is "silent skip, no placeholder"; FULL is "capacity
// exhausted, placeholder reason TOO_MANY_SLICES_CAPTURED".
typedef enum chased_slices_push_result {
  CHASED_SLICES_PUSH_INSERTED        = 0,
  CHASED_SLICES_PUSH_ALREADY_PRESENT = 1,
  CHASED_SLICES_PUSH_FULL            = 2,
} chased_slices_push_result_t;

static chased_slices_push_result_t
chased_slices_push(chased_slices_t* slices, uint64_t addr, uint32_t type_id, uint32_t len) {
  if (!slices) {
    LOG(1, "chased_slices_push: null %lld %d %d\n", addr, type_id, len);
    return CHASED_SLICES_PUSH_FULL;
  }
  uint32_t slices_len = slices->len;
  if (slices_len >= MAX_CHASED_SLICES) {
    LOG(3, "chased_slices_push: full %lld %d %d\n", addr, type_id, len);
    return CHASED_SLICES_PUSH_FULL;
  }
  for (int32_t i = slices_len - 1; i >= 0; i--) {
    if (slices->slices[i].addr == addr && slices->slices[i].type_id == type_id && slices->slices[i].len >= len) {
      return CHASED_SLICES_PUSH_ALREADY_PRESENT;
    }
  }
  slices->slices[slices_len] = (chased_slice_t){
      .addr = addr,
      .type_id = type_id,
      .len = len,
  };
  slices->len++;
  return CHASED_SLICES_PUSH_INSERTED;
}

#endif // __CHASED_SLICES_H__
