#ifndef __READ_INTO_BUFFER_H
#define __READ_INTO_BUFFER_H

#include "ktypes.h"

#include "bpf_builtins.h"
#include "bpf_telemetry.h"

#define BLK_SIZE (16)

#define STRINGIFY(a) #a

// The method is used to read the data buffer from the __sk_buf struct. Similar implementation as `read_into_buffer_skb`
// from http parsing, but uses a different constant (CLASSIFICATION_MAX_BUFFER).
#define READ_INTO_BUFFER(name, total_size, blk_size)                                                                \
    static __always_inline void read_into_buffer_##name(char *buffer, struct __sk_buff *skb, skb_info_t *info) {    \
        u64 offset = (u64)info->data_off;                                                                           \
        const u32 len = (total_size) < (skb->len - info->data_off) ? info->data_off + (total_size) : skb->len;      \
        unsigned i = 0;                                                                                             \
                                                                                                                    \
    _Pragma( STRINGIFY(unroll(total_size/blk_size)) )                                                               \
        for (; i < ((total_size) / (blk_size)); i++) {                                                              \
            if (offset + (blk_size) - 1 >= len) { break; }                                                          \
                                                                                                                    \
            bpf_skb_load_bytes_with_telemetry(skb, offset, &buffer[i * (blk_size)], (blk_size));                    \
            offset += (blk_size);                                                                                   \
        }                                                                                                           \
                                                                                                                    \
        const s64 left_payload = (s64)len - (s64)offset;                                                            \
        if (left_payload < 1) {                                                                                     \
            return;                                                                                                 \
        }                                                                                                           \
                                                                                                                    \
        s64 read_size = (blk_size) - 1;                                                                             \
        if (left_payload < read_size) {                                                                             \
            read_size = left_payload;                                                                               \
        }                                                                                                           \
                                                                                                                    \
        const s64 left_buffer = (s64)CLASSIFICATION_MAX_BUFFER - (s64)(i*(blk_size));                               \
        if (read_size <= left_buffer) {                                                                             \
            bpf_skb_load_bytes_with_telemetry(skb, offset, &buffer[i * (blk_size)], read_size);                     \
        }                                                                                                           \
        return;                                                                                                     \
    }                                                                                                               \

#endif
