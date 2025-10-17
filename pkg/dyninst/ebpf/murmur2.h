#ifndef __MURMUR2_H__
#define __MURMUR2_H__

#include "framing.h"

// Below code taken and lightly modified from
// https://github.com/parca-dev/parca-agent/blob/aa9289b868/bpf/unwinders/hash.h

// murmurhash2 from
// https://github.com/aappleby/smhasher/blob/92cf3702fcfaadc84eb7bef59825a23e0cd84f56/src/MurmurHash2.cpp

uint64_t hash_stack(stack_pcs_t* stack, int seed) {
  if (!stack) {
    return 0;
  }
  const uint64_t m = 0xc6a4a7935bd1e995LLU;
  const uint64_t r = 47;
  uint64_t len = stack->len;
  if (len > STACK_DEPTH) {
    len = STACK_DEPTH;
  }
  uint64_t hash = seed ^ (len * m);

  for (uint32_t i = 0; i < len; i++) {
    uint64_t k = stack->pcs[i];
    k *= m;
    k ^= k >> r;
    k *= m;
    hash ^= k;
  }
  return hash;
}

#endif // __MURMUR2_H__
