#ifndef _STRUCTS_RATE_LIMITER_H_
#define _STRUCTS_RATE_LIMITER_H_

#define RATE_LIMITER_COUNTER_MASK 0xffffllu

struct rate_limiter_ctx {
    /*
        data is representing both the `current_period` start
        in the first 6 bytes (basically current_period & ~0xff)
        and the counter in the last 2 bytes
    */
    u64 data;
};

static __always_inline struct rate_limiter_ctx new_rate_limiter(u64 now, u16 counter) {
    return (struct rate_limiter_ctx) {
        .data = (now & ~RATE_LIMITER_COUNTER_MASK) | counter,
    };
}

static __always_inline u64 get_current_period(struct rate_limiter_ctx *r) {
    return r->data & ~RATE_LIMITER_COUNTER_MASK;
}

static __always_inline u16 get_counter(struct rate_limiter_ctx *r) {
    return r->data & RATE_LIMITER_COUNTER_MASK;
}

static __always_inline void inc_counter(struct rate_limiter_ctx *r, u16 delta) {
    // this is an horrible hack, to keep the atomic property
    // we do an atomic add on the full data, worse case scenario
    // the current_period is increased by 256 nanoseconds
    __sync_fetch_and_add(&r->data, delta);
}

#endif /* _STRUCTS_RATE_LIMITER_H_ */
