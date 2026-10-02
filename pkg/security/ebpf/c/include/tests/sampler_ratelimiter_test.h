#ifndef _SAMPLER_RATELIMITER_TEST_H_
#define _SAMPLER_RATELIMITER_TEST_H_

#include "helpers/rate_limiter.h"
#include "baloum.h"

// default value of event_sampling.{open,connect,syscalls}.rate
#define SAMPLER_RL_TEST_RATE 500
#define SAMPLER_RL_TEST_PERIODS 5

static __attribute__((always_inline)) int check_sampler_limiter(u32 key, u32 other_key1, u32 other_key2) {
    for (int period = 0; period < SAMPLER_RL_TEST_PERIODS; period++) {
        // start a new period
        baloum_sleep(SEC_TO_NS(2));

        for (int i = 0; i < SAMPLER_RL_TEST_RATE; i++) {
            assert_not_zero(global_limiter_allow(key, SAMPLER_RL_TEST_RATE, 1),
                "sample not allowed which should be");
        }

        assert_zero(global_limiter_allow(key, SAMPLER_RL_TEST_RATE, 0),
            "sample allowed which should not be");
        assert_zero(global_limiter_allow(key, SAMPLER_RL_TEST_RATE, 1),
            "sample allowed which should not be");

        // an exhausted limiter doesn't block the samples of the other event types
        assert_not_zero(global_limiter_allow(other_key1, SAMPLER_RL_TEST_RATE, 0),
            "sample of another event type not allowed which should be");
        assert_not_zero(global_limiter_allow(other_key2, SAMPLER_RL_TEST_RATE, 0),
            "sample of another event type not allowed which should be");
    }
    return 0;
}

SEC("test/sampler_ratelimiter")
int test_sampler_ratelimiter() {
    if (check_sampler_limiter(OPEN_SAMPLE_LIMITER, CONNECT_SAMPLE_LIMITER, SYSCALLS_SAMPLE_LIMITER) != 0) {
        return -1;
    }
    if (check_sampler_limiter(CONNECT_SAMPLE_LIMITER, OPEN_SAMPLE_LIMITER, SYSCALLS_SAMPLE_LIMITER) != 0) {
        return -1;
    }
    if (check_sampler_limiter(SYSCALLS_SAMPLE_LIMITER, OPEN_SAMPLE_LIMITER, CONNECT_SAMPLE_LIMITER) != 0) {
        return -1;
    }
    return 1;
}

#endif /* _SAMPLER_RATELIMITER_TEST_H_ */
