#ifndef _RATE_LIMITER_H_
#define _RATE_LIMITER_H_

#include "maps.h"
#include "constants/macros.h"
#include "structs/rate_limiter.h"

__attribute__((always_inline)) u8 rate_limiter_reset_period(u64 now, struct rate_limiter_ctx *rate_ctx_p) {
    rate_ctx_p->current_period = now;
    rate_ctx_p->counter = 0;
    return 1;
}

__attribute__((always_inline)) u8 rate_limiter_allow_basic(u32 rate, u64 now, struct rate_limiter_ctx *rate_ctx_p, u64 delta) {
    if (delta > SEC_TO_NS(1)) { // if more than 1 sec ellapsed we reset the period
        return rate_limiter_reset_period(now, rate_ctx_p);
    }

    if (rate_ctx_p->counter >= rate) { // if we already allowed more than rate
        return 0;
    } else {
        return 1;
    }
}

__attribute__((always_inline)) u8 rate_limiter_allow_gen(struct rate_limiter_ctx *rate_ctx_p, u32 rate, u64 now, u8 should_count) {
    if (now < rate_ctx_p->current_period) { // this should never happen, ignore
        return 0;
    }
    u64 delta = now - rate_ctx_p->current_period;
    u8 allow = rate_limiter_allow_basic(rate, now, rate_ctx_p, delta);
    if (allow && should_count) {
        __sync_fetch_and_add(&rate_ctx_p->counter, 1);
    }
    return (allow);
}

// For now the generic rate is staticaly defined
// TODO: put it configurable
#define GENERIC_RATE_LIMITER_RATE 100

__attribute__((always_inline)) u8 rate_limiter_allow(u32 pid, u64 now, u8 should_count) {
    if (now == 0) {
        now = bpf_ktime_get_ns();
    }
    if (pid == 0) {
        pid = bpf_get_current_pid_tgid() >> 32;
    }

    struct rate_limiter_ctx *rate_ctx_p = bpf_map_lookup_elem(&rate_limiters, &pid);
    if (rate_ctx_p == NULL) {
        struct rate_limiter_ctx rate_ctx = {
            .current_period = now,
            .counter = should_count,
        };
        bpf_map_update_elem(&rate_limiters, &pid, &rate_ctx, BPF_ANY);
        return 1;
    }

    u32 rate = GENERIC_RATE_LIMITER_RATE;
    return rate_limiter_allow_gen(rate_ctx_p, rate, now, should_count);
}
#define rate_limiter_allow_simple() rate_limiter_allow(0, 0, 1)

__attribute__((always_inline)) u8 activity_dump_rate_limiter_allow(u32 rate, u64 cookie, u64 now, u8 should_count) {
    if (now == 0) {
        now = bpf_ktime_get_ns();
    }

    struct rate_limiter_ctx *rate_ctx_p = bpf_map_lookup_elem(&activity_dump_rate_limiters, &cookie);
    if (rate_ctx_p == NULL) {
        struct rate_limiter_ctx rate_ctx = {
            .current_period = now,
            .counter = should_count,
        };
        bpf_map_update_elem(&activity_dump_rate_limiters, &cookie, &rate_ctx, BPF_ANY);
        return 1;
    }

    return rate_limiter_allow_gen(rate_ctx_p, rate, now, should_count);
}

#endif /* _RATE_LIMITER_H_ */
