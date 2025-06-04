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

  // The currently used clang compiler along with older kernel verifiers doesn't
  // handle a direct loop well, hitting verifier complexity limits. We generate
  // unwound loop trading code-size for portability
  uint64_t k;
  _Static_assert(STACK_DEPTH == 511, "update static loop below when changing STACK_DEPTH");
#define ITER_1(i) \
  if (i >= len) { \
    return hash; \
  } \
  k = stack->pcs[i]; \
  k *= m; \
  k ^= k >> r; \
  k *= m; \
  hash ^= k; \
  hash *= m;
#define ITER_2(i) ITER_1(i) ITER_1(i + 1)
#define ITER_4(i) ITER_2(i) ITER_2(i + 2)
#define ITER_8(i) ITER_4(i) ITER_4(i + 4)
#define ITER_16(i) ITER_8(i) ITER_8(i + 8)
#define ITER_32(i) ITER_16(i) ITER_16(i + 16)
#define ITER_64(i) ITER_32(i) ITER_32(i + 32)
#define ITER_128(i) ITER_64(i) ITER_64(i + 64)
#define ITER_256(i) ITER_128(i) ITER_128(i + 128)

  ITER_256(0);
  ITER_128(0b100000000);
  ITER_64 (0b110000000);
  ITER_32 (0b111000000);
  ITER_16 (0b111100000);
  ITER_8  (0b111110000);
  ITER_4  (0b111111000);
  ITER_2  (0b111111100);
  ITER_1  (0b111111110);

#undef ITER_1
#undef ITER_2
#undef ITER_4
#undef ITER_8
#undef ITER_16
#undef ITER_32
#undef ITER_64
#undef ITER_128
#undef ITER_256
#undef ITER_512

  return hash;
}

#endif // __MURMUR2_H__
