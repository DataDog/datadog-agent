#ifndef _STRUCTS_RATE_LIMITER_H_
#define _STRUCTS_RATE_LIMITER_H_

struct rate_limiter_ctx {
    /*
        data is representing both the `current_period` start
        in the first 7 bytes (basically current_period & ~0xff)
        and the counter in the last byte
    */
    u64 data;
};

struct rate_limiter_ctx __attribute__((always_inline)) new_rate_limiter(u64 now, u8 counter) {
    return (struct rate_limiter_ctx) {
        .data = (now & ~((u64)0xff)) | counter,
    };
}

u64 __attribute__((always_inline)) get_current_period(struct rate_limiter_ctx *r) {
    return r->data & ~((u64)0xff);
}

u8 __attribute__((always_inline)) get_counter(struct rate_limiter_ctx *r) {
    return r->data & 0xff;
}

void __attribute__((always_inline)) inc_counter(struct rate_limiter_ctx *r, u8 delta) {
    u8 new_counter = get_counter(r) + delta;
    r->data = (r->data & ~0xff) | new_counter;
}

#endif /* _STRUCTS_RATE_LIMITER_H_ */
