#ifndef _RATE_LIMITER_H_
#define _RATE_LIMITER_H_

#include "maps.h"
#include "constants/macros.h"
#include "structs/rate_limiter.h"

enum rate_limiter_algo_ids
{
    RL_ALGO_BASIC = 0,
    RL_ALGO_BASIC_HALF,
    RL_ALGO_DECREASING_DROPRATE,
    RL_ALGO_INCREASING_DROPRATE,
    RL_ALGO_TOTAL_NUMBER,
};

__attribute__((always_inline)) u8 rate_limiter_reset_period(u64 now, struct rate_limiter_ctx *rate_ctx_p) {
    rate_ctx_p->current_period = now;
    rate_ctx_p->counter = 0;
#ifndef __BALOUM__ // do not change algo during unit tests
    rate_ctx_p->algo_id = now % RL_ALGO_TOTAL_NUMBER;
#endif /* __BALOUM__ */
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

__attribute__((always_inline)) u8 rate_limiter_allow_basic_half(u32 rate, u64 now, struct rate_limiter_ctx *rate_ctx_p, u64 delta) {
    if (delta > SEC_TO_NS(1) / 2) { // if more than 0.5 sec ellapsed we reset the period
        return rate_limiter_reset_period(now, rate_ctx_p);
    }

    if (rate_ctx_p->counter >= rate / 2) { // if we already allowed more than rate / 2
        return 0;
    } else {
        return 1;
    }
}

__attribute__((always_inline)) u8 rate_limiter_allow_decreasing_droprate(u32 rate, u64 now, struct rate_limiter_ctx *rate_ctx_p, u64 delta) {
    if (delta > SEC_TO_NS(1)) { // if more than 1 sec ellapsed we reset the period
        return rate_limiter_reset_period(now, rate_ctx_p);
    }

    if (rate_ctx_p->counter >= rate) { // if we already allowed more than rate
        return 0;
    } else if (rate_ctx_p->counter < (rate / 4)) { // first 1/4 is not rate limited
        return 1;
    }

    // if we are between rate / 4 and rate, apply a decreasing rate of:
    // (counter * 100) / (rate) %
    else if (now % ((rate_ctx_p->counter * 100) / rate) == 0) {
        return 1;
    }
    return 0;
}

__attribute__((always_inline)) u8 rate_limiter_allow_increasing_droprate(u32 rate, u64 now, struct rate_limiter_ctx *rate_ctx_p, u64 delta) {
    if (delta > SEC_TO_NS(1)) { // if more than 1 sec ellapsed we reset the period
        return rate_limiter_reset_period(now, rate_ctx_p);
    }

    if (rate_ctx_p->counter >= rate) { // if we already allowed more than rate
        return 0;
    } else if (rate_ctx_p->counter < (rate / 4)) { // first 1/4 is not rate limited
        return 1;
    }

    // if we are between rate / 4 and rate, apply an increasing rate of:
    // 100 - ((counter * 100) / (rate)) %
    else if (now % (100 - ((rate_ctx_p->counter * 100) / rate)) == 0) {
        return 1;
    }
    return 0;
}

__attribute__((always_inline)) u8 rate_limiter_allow_gen(struct rate_limiter_ctx *rate_ctx_p, u32 rate, u64 now, u8 should_count) {
    if (now < rate_ctx_p->current_period) { // this should never happen, ignore
        return 0;
    }
    u64 delta = now - rate_ctx_p->current_period;

    u8 allow;
    switch (rate_ctx_p->algo_id) {
    case RL_ALGO_BASIC:
        allow = rate_limiter_allow_basic(rate, now, rate_ctx_p, delta);
        break;
    case RL_ALGO_BASIC_HALF:
        allow = rate_limiter_allow_basic_half(rate, now, rate_ctx_p, delta);
        break;
    case RL_ALGO_DECREASING_DROPRATE:
        allow = rate_limiter_allow_decreasing_droprate(rate, now, rate_ctx_p, delta);
        break;
    case RL_ALGO_INCREASING_DROPRATE:
        allow = rate_limiter_allow_increasing_droprate(rate, now, rate_ctx_p, delta);
        break;
    default: // should never happen, ignore
        return 0;
    }

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
            .algo_id = now % RL_ALGO_TOTAL_NUMBER,
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
            .algo_id = now % RL_ALGO_TOTAL_NUMBER,
        };
        bpf_map_update_elem(&activity_dump_rate_limiters, &cookie, &rate_ctx, BPF_ANY);
        return 1;
    }

    return rate_limiter_allow_gen(rate_ctx_p, rate, now, should_count);
}

#endif /* _RATE_LIMITER_H_ */
