#ifndef __KAFKA_BUFFER_H
#define __KAFKA_BUFFER_H

#include <linux/err.h>
#include "kafka-types.h"


// This function is used for the socket-filter Kafka monitoring
static __always_inline void read_into_buffer_skb(char *buffer, struct __sk_buff *skb, skb_info_t *info) {
u64 offset = (u64)info->data_off;

#define BLK_SIZE (16)
    const u32 len = KAFKA_BUFFER_SIZE < (skb->len - (u32)offset) ? (u32)offset + KAFKA_BUFFER_SIZE : skb->len;

    unsigned i = 0;

#pragma unroll(KAFKA_BUFFER_SIZE / BLK_SIZE)
    for (; i < (KAFKA_BUFFER_SIZE / BLK_SIZE); i++) {
        if (offset + BLK_SIZE - 1 >= len) { break; }

        bpf_skb_load_bytes(skb, offset, &buffer[i * BLK_SIZE], BLK_SIZE);
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
        bpf_skb_load_bytes(skb, offset, buf, 15);
    } else if (offset + 13 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 14);
    } else if (offset + 12 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 13);
    } else if (offset + 11 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 12);
    } else if (offset + 10 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 11);
    } else if (offset + 9 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 10);
    } else if (offset + 8 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 9);
    } else if (offset + 7 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 8);
    } else if (offset + 6 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 7);
    } else if (offset + 5 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 6);
    } else if (offset + 4 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 5);
    } else if (offset + 3 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 4);
    } else if (offset + 2 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 3);
    } else if (offset + 1 < len) {
        bpf_skb_load_bytes(skb, offset, buf, 2);
    } else if (offset < len) {
        bpf_skb_load_bytes(skb, offset, buf, 1);
    }
}

#endif
