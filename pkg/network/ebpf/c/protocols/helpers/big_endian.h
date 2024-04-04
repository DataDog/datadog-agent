#ifndef __BIG_ENDIAN_HELPERS_H
#define __BIG_ENDIAN_HELPERS_H

#include "bpf_endian.h"

#define identity_transformer(x) (x)

// Template for read_big_endian_{s16, s32} methods. The function gets skb, offset and an out parameter of the relevant
// type, verifies we do not exceed the packet's boundaries, and reads the relevant number from the packet. Eventually
// we are converting the little-endian (default by the read) to big-endian. Return false if we exceeds boundaries, true
// otherwise.
#define READ_BIG_ENDIAN(type, transformer)                                                                  \
    static __always_inline __maybe_unused bool read_big_endian_##type(struct __sk_buff *skb, u32 offset, type *out) {      \
        if (offset + sizeof(type) > skb->len) {                                                             \
            return false;                                                                                   \
        }                                                                                                   \
        type val;                                                                                           \
        bpf_memset(&val, 0, sizeof(type));                                                                  \
        bpf_skb_load_bytes_with_telemetry(skb, offset, &val, sizeof(type));                                 \
        *out = transformer(val);                                                                            \
        return true;                                                                                        \
    }

READ_BIG_ENDIAN(s32, bpf_ntohl);
READ_BIG_ENDIAN(s16, bpf_ntohs);
READ_BIG_ENDIAN(s8, identity_transformer);

#endif
