#ifndef __HTTP_BUFFER_H
#define __HTTP_BUFFER_H

#include <linux/err.h>
#include "bpf_builtins.h"
#include "bpf_telemetry.h"
#include "http-types.h"

// This function reads a constant number of bytes into the fragment buffer of the http
// transaction object, and returns the number of bytes of the valid data. The number of
// bytes are used in userspace to zero out the garbage we may have read into the buffer.
//
// This function is used for the uprobe-based HTTPS monitoring (eg. OpenSSL, GnuTLS etc)
static __always_inline void read_into_buffer(char *buffer, char *data, size_t data_size) {
    bpf_memset(buffer, 0, HTTP_BUFFER_SIZE);

    // we read HTTP_BUFFER_SIZE-1 bytes to ensure that the string is always null terminated
    if (bpf_probe_read_user_with_telemetry(buffer, HTTP_BUFFER_SIZE - 1, data) < 0) {
// note: arm64 bpf_probe_read_user() could page fault if the HTTP_BUFFER_SIZE overlap a page
#pragma unroll(HTTP_BUFFER_SIZE - 1)
        for (int i = 0; i < HTTP_BUFFER_SIZE - 1; i++) {
            bpf_probe_read_user(&buffer[i], 1, &data[i]);
            if (buffer[i] == 0) {
                return;
            }
        }
    }
}


// This function is used for the socket-filter HTTP monitoring
static __always_inline void read_into_buffer_skb(char *buffer, struct __sk_buff *skb, skb_info_t *info) {
    u64 offset = (u64)info->data_off;

#define BLK_SIZE (16)
    const u32 len = HTTP_BUFFER_SIZE < (skb->len - (u32)offset) ? (u32)offset + HTTP_BUFFER_SIZE : skb->len;

    unsigned i = 0;

#pragma unroll(HTTP_BUFFER_SIZE / BLK_SIZE)
    for (; i < (HTTP_BUFFER_SIZE / BLK_SIZE); i++) {
        if (offset + BLK_SIZE - 1 >= len) { break; }

        bpf_skb_load_bytes_with_telemetry(skb, offset, &buffer[i * BLK_SIZE], BLK_SIZE);
        offset += BLK_SIZE;
    }

    // This part is very hard to write in a loop and unroll it.
    // Indeed, mostly because of older kernel verifiers, we want to make sure the offset into the buffer is not
    // stored on the stack, so that the verifier is able to verify that we're not doing out-of-bound on
    // the stack.
    // Basically, we should get a register from the code block above containing an fp relative address. As
    // we are doing `buffer[0]` here, there is not dynamic computation on that said register after this,
    // and thus the verifier is able to ensure that we are in-bound.
    void *buf = &buffer[i * BLK_SIZE];
    if (offset + 14 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 15);
    } else if (offset + 13 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 14);
    } else if (offset + 12 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 13);
    } else if (offset + 11 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 12);
    } else if (offset + 10 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 11);
    } else if (offset + 9 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 10);
    } else if (offset + 8 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 9);
    } else if (offset + 7 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 8);
    } else if (offset + 6 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 7);
    } else if (offset + 5 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 6);
    } else if (offset + 4 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 5);
    } else if (offset + 3 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 4);
    } else if (offset + 2 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 3);
    } else if (offset + 1 < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 2);
    } else if (offset < len) {
        bpf_skb_load_bytes_with_telemetry(skb, offset, buf, 1);
    }
}

#endif
