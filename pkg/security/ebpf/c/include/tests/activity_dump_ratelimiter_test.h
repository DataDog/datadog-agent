#ifndef _ACTIVITY_DUMP_RATELIMITER_TEST_H_
#define _ACTIVITY_DUMP_RATELIMITER_TEST_H_

#include "helpers/activity_dump.h"
#include "helpers/utils.h"
#include "baloum.h"

#define AD_RL_TEST_RATE 500
#define NUMBER_OF_PERIOD_PER_TEST 10

SEC("test/ad_ratelimiter_basic")
int test_ad_ratelimiter_basic()
{
    u64 now = bpf_ktime_get_ns();

    struct activity_dump_config config;
    config.events_rate = AD_RL_TEST_RATE;

    struct activity_dump_rate_limiter_ctx ctx;
    ctx.counter = 0;
    ctx.current_period = now;
    ctx.algo_id = RL_ALGO_BASIC; // force algo basic
    u64 cookie = 0;
    bpf_map_update_elem(&activity_dump_rate_limiters, &cookie, &ctx, BPF_ANY);

    for (int period_cpt = 0; period_cpt < NUMBER_OF_PERIOD_PER_TEST; period_cpt++, now += SEC_TO_NS(2)) {
        assert_not_zero(activity_dump_rate_limiter_allow(&config, cookie, now, 0),
                        "event not allowed which should be");
        for (int i = 0; i < AD_RL_TEST_RATE; i++) {
            assert_not_zero(activity_dump_rate_limiter_allow(&config, cookie, now + i, 1),
                            "event not allowed which should be");
        }

        assert_zero(activity_dump_rate_limiter_allow(&config, cookie, now, 0),
                    "event allowed which should not be");
        for (int i = 0; i < AD_RL_TEST_RATE; i++) {
            assert_zero(activity_dump_rate_limiter_allow(&config, cookie, now + i, 1),
                        "event allowed which should not be");
        }
        assert_zero(activity_dump_rate_limiter_allow(&config, cookie, now, 0),
                    "event allowed which should not be");
    }
    return 0;
}

SEC("test/ad_ratelimiter_basic_half")
int test_ad_ratelimiter_basic_half()
{
    u64 now = bpf_ktime_get_ns();

    struct activity_dump_config config;
    config.events_rate = AD_RL_TEST_RATE;

    struct activity_dump_rate_limiter_ctx ctx;
    ctx.counter = 0;
    ctx.current_period = now;
    ctx.algo_id = RL_ALGO_BASIC_HALF; // force algo basic half
    u64 cookie = 0;
    bpf_map_update_elem(&activity_dump_rate_limiters, &cookie, &ctx, BPF_ANY);

    for (int period_cpt = 0; period_cpt < NUMBER_OF_PERIOD_PER_TEST; period_cpt++, now += SEC_TO_NS(1)) {
        assert_not_zero(activity_dump_rate_limiter_allow(&config, cookie, now, 0),
                        "event not allowed which should be");
        for (int i = 0; i < AD_RL_TEST_RATE / 2; i++) {
            assert_not_zero(activity_dump_rate_limiter_allow(&config, cookie, now + i, 1),
                            "event not allowed which should be");
        }

        assert_zero(activity_dump_rate_limiter_allow(&config, cookie, now, 0),
                    "event allowed which should not be");
        for (int i = 0; i < AD_RL_TEST_RATE / 2; i++) {
            assert_zero(activity_dump_rate_limiter_allow(&config, cookie, now + i, 1),
                        "event allowed which should not be");
        }
        assert_zero(activity_dump_rate_limiter_allow(&config, cookie, now, 0),
                    "event allowed which should not be");
    }
    return 0;
}

__attribute__((always_inline)) int test_ad_ratelimiter_variable_droprate(int algo)
{
    u64 now = bpf_ktime_get_ns();

    struct activity_dump_config config;
    config.events_rate = AD_RL_TEST_RATE;

    struct activity_dump_rate_limiter_ctx ctx;
    ctx.counter = 0;
    ctx.current_period = now;
    ctx.algo_id = algo; // force algo
    u64 cookie = 0;
    bpf_map_update_elem(&activity_dump_rate_limiters, &cookie, &ctx, BPF_ANY);

    for (int period_cpt = 0; period_cpt < NUMBER_OF_PERIOD_PER_TEST; period_cpt++, now += SEC_TO_NS(2)) {
    assert_not_zero(activity_dump_rate_limiter_allow(&config, cookie, now, 0),
                    "event not allowed which should be");
    for (int i = 0; i < AD_RL_TEST_RATE / 4; i++) {
        assert_not_zero(activity_dump_rate_limiter_allow(&config, cookie, now + i, 1),
                        "event not allowed which should be");
    }

    int total_allowed = 0;
    for (int i = 0; i < AD_RL_TEST_RATE * 10; i++) {
        if (activity_dump_rate_limiter_allow(&config, cookie, now + i, 1)) {
            total_allowed++;
        }
    }
    assert_greater_than(total_allowed, AD_RL_TEST_RATE * 3 / 4, "nope");
    assert_lesser_than(total_allowed, AD_RL_TEST_RATE / 10, "nope");
    }
    return 0;
}

SEC("test/ad_ratelimiter_decreasing_droprate")
int test_ad_ratelimiter_decreasing_droprate()
{
    return test_ad_ratelimiter_variable_droprate(RL_ALGO_DECREASING_DROPRATE);
}

SEC("test/ad_ratelimiter_increasing_droprate")
int test_ad_ratelimiter_increasing_droprate()
{
    return test_ad_ratelimiter_variable_droprate(RL_ALGO_INCREASING_DROPRATE);
}

#endif /* _ACTIVITY_DUMP_RATELIMITER_TEST_H_ */
