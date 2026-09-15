#ifndef _APPROVERS_TEST_H
#define _APPROVERS_TEST_H

#include "helpers/approvers.h"
#include "baloum.h"

// A flag approver holds one bit per approved value in a 64 bit mask, and user
// space sets bit `value % 64`, so every shift from 0 to 63 has to round trip.
#define assert_flag_approver_bit(bit)                                                                          \
    {                                                                                                          \
        struct u64_flags_filter_t filter = {                                                                   \
            .flags = (u64)1 << (bit),                                                                          \
            .is_set = 1,                                                                                       \
        };                                                                                                     \
        assert_equals(flag_approver(&filter, EVENT_SOCKET, (bit)), APPROVED,                                   \
                      "value in the mask should be approved");                                                 \
        assert_equals(flag_approver(&filter, EVENT_SOCKET, ((bit) + 1) % 64), DISCARDED,                       \
                      "value outside the mask should be discarded");                                           \
    }

SEC("test/flag_approver_low_bits")
int test_flag_approver_low_bits() {
#pragma unroll
    for (int bit = 0; bit < 32; bit++) {
        assert_flag_approver_bit(bit);
    }

    return 1;
}

// AF_VSOCK, BPF_LINK_DETACH, PR_SET_NO_NEW_PRIVS and SO_ATTACH_BPF are among
// the values that land in the upper half of the mask.
SEC("test/flag_approver_high_bits")
int test_flag_approver_high_bits() {
#pragma unroll
    for (int bit = 32; bit < 64; bit++) {
        assert_flag_approver_bit(bit);
    }

    return 1;
}

SEC("test/flag_approver_unset")
int test_flag_approver_unset() {
    struct u64_flags_filter_t filter = {
        .flags = ~(u64)0,
        .is_set = 0,
    };

    // an approver that was never installed approves nothing, whatever it holds
    assert_equals(flag_approver(&filter, EVENT_SOCKET, 0), DISCARDED, "unset filter should discard");
    assert_equals(flag_approver(NULL, EVENT_SOCKET, 0), DISCARDED, "missing filter should discard");

    return 1;
}

#endif /* _APPROVERS_TEST_H */
