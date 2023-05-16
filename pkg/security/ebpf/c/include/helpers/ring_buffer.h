#ifndef _HELPERS_RING_BUFFER_H_
#define _HELPERS_RING_BUFFER_H_

#include "maps.h"
#include "constants/custom.h"

u32 __attribute__((always_inline)) rb_get_tail_length(struct ring_buffer_ctx *rb_ctx) {
    rb_ctx->write_cursor %= RING_BUFFER_SIZE;
    return RING_BUFFER_SIZE - rb_ctx->write_cursor;
}

void __attribute__((always_inline)) rb_push_str(struct ring_buffer_t *rb, struct ring_buffer_ctx *rb_ctx, char *str, u32 const_len) {
    rb_ctx->write_cursor %= RING_BUFFER_SIZE;
    if (rb_ctx->write_cursor + const_len <= RING_BUFFER_SIZE) {
        long len = bpf_probe_read_str(&rb->buffer[rb_ctx->write_cursor], const_len, str);
        if (len > 0) {
            // bpf_probe_read_str will set the last byte to NULL, so remove 1 from the total len so that it gets overwritten on the next push
            rb_ctx->write_cursor = (rb_ctx->write_cursor + len - 1) % RING_BUFFER_SIZE;
            rb_ctx->len += (len - 1);
        }
    }
}

void __attribute__((always_inline)) rb_push_watermark(struct ring_buffer_t *rb, struct ring_buffer_ctx *rb_ctx) {
#pragma unroll
    for (unsigned int i = 0; i < sizeof(rb_ctx->watermark); i++) {
        rb->buffer[rb_ctx->write_cursor++ % RING_BUFFER_SIZE] = *(((char *)&rb_ctx->watermark) + i);
    }
    rb_ctx->write_cursor %= RING_BUFFER_SIZE;
    rb_ctx->len += sizeof(rb_ctx->watermark);
}

void __attribute__((always_inline)) rb_push_char(struct ring_buffer_t *rb, struct ring_buffer_ctx *rb_ctx, char c) {
    rb->buffer[rb_ctx->write_cursor++ % RING_BUFFER_SIZE] = c;
    rb_ctx->write_cursor %= RING_BUFFER_SIZE;
    rb_ctx->len += 1;
}

void __attribute__((always_inline)) rb_cleanup_ctx(struct ring_buffer_ctx *rb_ctx) {
    rb_ctx->write_cursor = rb_ctx->read_cursor;
    rb_ctx->watermark = 0;
    rb_ctx->len = 0;
}

#endif