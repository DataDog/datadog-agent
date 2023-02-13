#ifndef __READ_INTO_BUFFER_H
#define __READ_INTO_BUFFER_H

#include "ktypes.h"

#include "bpf_builtins.h"
#include "bpf_telemetry.h"

#define BLK_SIZE (16)

#define STRINGIFY(a) #a

// The method is used to read the data buffer from the TCP segment data up to `total_size` bytes.
#define READ_INTO_BUFFER(name, total_size, blk_size)                                                                \
    static __always_inline void read_into_buffer_##name(char *buffer, struct __sk_buff *skb, u32 offset) {          \
        const u32 end = (total_size) < (skb->len - offset) ? offset + (total_size) : skb->len;                      \
        unsigned i = 0;                                                                                             \
                                                                                                                    \
    _Pragma( STRINGIFY(unroll(total_size/blk_size)) )                                                               \
        for (; i < ((total_size) / (blk_size)); i++) {                                                              \
            if (offset + (blk_size) - 1 >= end) { break; }                                                          \
                                                                                                                    \
            bpf_skb_load_bytes_with_telemetry(skb, offset, &buffer[i * (blk_size)], (blk_size));                    \
            offset += (blk_size);                                                                                   \
        }                                                                                                           \
                                                                                                                    \
        /* Calculating the remaining bytes to read. If we have none, then we abort. */                              \
        const s64 left_payload = (s64)end - (s64)offset;                                                            \
        if (left_payload < 1) {                                                                                     \
            return;                                                                                                 \
        }                                                                                                           \
                                                                                                                    \
        /* The maximum that we can read is (blk_size) - 1. Checking (to please the verifier) that we read no more */\
        /* than the allowed max size. */                                                                            \
        const u64 read_size = left_payload < (blk_size) - 1 ? left_payload : (blk_size) - 1;                        \
                                                                                                                    \
        /* Calculating the absolute size from the allocated buffer, that was left empty, again to please the */     \
        /* verifier so it can be assured we are not exceeding the memory limits. */                                 \
        const u64 left_buffer = (s64)(total_size) < (s64)(i*(blk_size)) ? 0 : total_size - i*(blk_size);            \
        if (read_size <= left_buffer) {                                                                             \
            bpf_skb_load_bytes_with_telemetry(skb, offset, &buffer[i * (blk_size)], read_size);                     \
        }                                                                                                           \
        return;                                                                                                     \
    }                                                                                                               \

#endif
