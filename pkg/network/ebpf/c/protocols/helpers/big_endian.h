#ifndef __BIG_ENDIAN_HELPERS_H
#define __BIG_ENDIAN_HELPERS_H

#include "bpf_endian.h"

#define identity_transformer(x) (x)

static __always_inline long bpf_sk_msg_load_bytes(struct sk_msg_md *msg, u32 offset, void *to, u32 len)
{
    long err = bpf_msg_pull_data(msg, offset, offset + len, 0);
    if (err < 0) {
        return err;
    }

    void *data = msg->data;
    void *data_end = msg->data_end;
    if (data + len > data_end) {
        return -1;
    }

    return bpf_probe_read_kernel(to, len, data);
}

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

#define READ_BIG_ENDIAN_USER(type, transformer)                                                                                   \
    static __always_inline __maybe_unused bool read_big_endian_user_##type(const void *buf, u32 buflen, u32 offset, type *out) {  \
        if (offset + sizeof(type) > buflen) {                                                                                     \
            return false;                                                                                                         \
        }                                                                                                                         \
        type val;                                                                                                                 \
        bpf_memset(&val, 0, sizeof(type));                                                                                        \
        bpf_probe_read_user(&val, sizeof(type), buf + offset);                                                                    \
        *out = transformer(val);                                                                                                  \
        return true;                                                                                                              \
    }

READ_BIG_ENDIAN_USER(s32, bpf_ntohl);
READ_BIG_ENDIAN_USER(s16, bpf_ntohs);
READ_BIG_ENDIAN_USER(s8, identity_transformer);

#define READ_BIG_ENDIAN_SK_MSG(type, transformer)                                                                  \
    static __always_inline __maybe_unused bool read_big_endian_sk_msg_##type(struct sk_msg_md *msg, u32 offset, type *out) {      \
        if (offset + sizeof(type) > msg->size) {                                                             \
            return false;                                                                                   \
        }                                                                                                   \
        type val;                                                                                           \
        bpf_memset(&val, 0, sizeof(type));                                                                  \
        bpf_sk_msg_load_bytes(msg, offset, &val, sizeof(type));                                 \
        *out = transformer(val);                                                                            \
        return true;                                                                                        \
    }

READ_BIG_ENDIAN(s32, bpf_ntohl);
READ_BIG_ENDIAN(s16, bpf_ntohs);
READ_BIG_ENDIAN(s8, identity_transformer);

READ_BIG_ENDIAN_SK_MSG(s32, bpf_ntohl);
READ_BIG_ENDIAN_SK_MSG(s16, bpf_ntohs);
READ_BIG_ENDIAN_SK_MSG(s8, identity_transformer);

#endif
