#ifndef _ACTIVITY_DUMP_RATELIMITER_TEST_H_
#define _ACTIVITY_DUMP_RATELIMITER_TEST_H_

#include "helpers/activity_dump.h"
#include "helpers/utils.h"
#include "baloum.h"

#define AD_RL_TEST_RATE 500
#define NUMBER_OF_PERIOD_PER_TEST 10

SEC("test/ad_ratelimiter")
int test_ad_ratelimiter() {
    u64 now = bpf_ktime_get_ns();

    u32 rate = AD_RL_TEST_RATE;

    struct rate_limiter_ctx ctx = new_rate_limiter(now, 0);
    u64 cookie = 0;
    bpf_map_update_elem(&activity_dump_rate_limiters, &cookie, &ctx, BPF_ANY);

    for (int period_cpt = 0; period_cpt < NUMBER_OF_PERIOD_PER_TEST; period_cpt++, now += SEC_TO_NS(2)) {
        assert_not_zero(activity_dump_rate_limiter_allow(rate, cookie, now, 0),
            "event not allowed which should be");
        for (int i = 0; i < AD_RL_TEST_RATE; i++) {
            assert_not_zero(activity_dump_rate_limiter_allow(rate, cookie, now + i, 1),
                "event not allowed which should be");
        }

        assert_zero(activity_dump_rate_limiter_allow(rate, cookie, now, 0),
            "event allowed which should not be");
        for (int i = 0; i < AD_RL_TEST_RATE; i++) {
            assert_zero(activity_dump_rate_limiter_allow(rate, cookie, now + i, 1),
                "event allowed which should not be");
        }
        assert_zero(activity_dump_rate_limiter_allow(rate, cookie, now, 0),
            "event allowed which should not be");
    }
    return 1;
}

#endif /* _ACTIVITY_DUMP_RATELIMITER_TEST_H_ */
