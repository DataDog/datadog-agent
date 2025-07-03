#ifndef __BINARY_SEARCH_H__
#define __BINARY_SEARCH_H__

#include "bpf_helpers.h"
#include "compiler.h"
#include "ktypes.h"

typedef struct binary_search_ctx {
  uint32_t left;
  uint32_t right;
} binary_search_ctx_t;

#define LOG2_1(n) ((n) >= (1ULL << 1) ? 1 : 0)
#define LOG2_2(n) ((n) >= (1ULL << 2) ? 2 + LOG2_1((n) >> 2) : LOG2_1(n))
#define LOG2_4(n) ((n) >= (1ULL << 4) ? 4 + LOG2_2((n) >> 4) : LOG2_2(n))
#define LOG2_8(n) ((n) >= (1ULL << 8) ? 8 + LOG2_4((n) >> 8) : LOG2_4(n))
#define LOG2_16(n) ((n) >= (1ULL << 16) ? 16 + LOG2_8((n) >> 16) : LOG2_8(n))
#define LOG2_32(n) ((n) >= (1ULL << 32) ? 32 + LOG2_16((n) >> 32) : LOG2_16(n))

#define LOG2(n) LOG2_32((uint64_t)(n))
#define CEIL_LOG2(n) (LOG2(n) + ((n) > (1ULL << LOG2(n)) ? 1 : 0))

#define DEFINE_BINARY_SEARCH(prefix, target_type, target_name, array_name,     \
                             bound_name)                                       \
  typedef struct prefix##_by_##target_name##_ctx {                             \
    target_type target_##target_name;                                          \
    binary_search_ctx_t search_ctx;                                            \
  } prefix##_by_##target_name##_ctx_t;                                         \
                                                                               \
  static long prefix##_by_##target_name##_loop(                                \
      __maybe_unused unsigned long _, void* ctx) {                             \
    prefix##_by_##target_name##_ctx_t* search_ctx =                            \
        (prefix##_by_##target_name##_ctx_t*)ctx;                               \
    binary_search_ctx_t* bin_ctx = &search_ctx->search_ctx;                    \
    uint32_t size = (bin_ctx->right - bin_ctx->left);                          \
    uint32_t mid = bin_ctx->left + (size / 2);                                 \
    if (mid >= bound_name) {                                                   \
      return 1;                                                                \
    }                                                                          \
    uint32_t* value = bpf_map_lookup_elem(&array_name, &mid);                  \
    if (!value) {                                                              \
      return 1;                                                                \
    }                                                                          \
    if (*value < search_ctx->target_##target_name) {                           \
      bin_ctx->left = mid + 1;                                                 \
    } else if (*value == search_ctx->target_##target_name) {                   \
      bin_ctx->left = mid;                                                     \
      bin_ctx->right = mid;                                                    \
    } else {                                                                   \
      bin_ctx->right = mid;                                                    \
    }                                                                          \
    if (bin_ctx->left == bin_ctx->right) {                                     \
      return 1;                                                                \
    }                                                                          \
    return 0;                                                                  \
  }                                                                            \
                                                                               \
  uint32_t prefix##_by_##target_name(target_type target_name) {                \
    prefix##_by_##target_name##_ctx_t ctx = {                                  \
        .target_##target_name = target_name,                                   \
        .search_ctx =                                                          \
            {                                                                  \
                .left = 0,                                                     \
                .right = bound_name,                                           \
            },                                                                 \
    };                                                                         \
    const int n = CEIL_LOG2(bound_name);                                       \
    bpf_loop(n, prefix##_by_##target_name##_loop, &ctx, 0);                    \
    return ctx.search_ctx.left;                                                \
  }

#endif // __BINARY_SEARCH_H__
