#ifndef _APPROVERS_TEST_H
#define _APPROVERS_TEST_H

#include "helpers/approvers.h"
#include "baloum.h"

SEC("test/flag_approver")
int test_flag_approver() {
    struct u64_flags_filter_t filter = {
        .flags = ((u64)1 << 3) | ((u64)1 << 31) | ((u64)1 << 62),
        .is_set = 1,
    };

    // volatile keeps the shift count a runtime value, as it is for a syscall argument
    volatile u64 value = 3;
    assert_equals(flag_approver(&filter, EVENT_PRCTL, value), APPROVED, "value 3 should be approved");

    value = 31;
    assert_equals(flag_approver(&filter, EVENT_PRCTL, value), APPROVED, "value 31 should be approved");

    value = 62;
    assert_equals(flag_approver(&filter, EVENT_PRCTL, value), APPROVED, "value 62 should be approved");

    // userspace sets bit (value % 64)
    value = 126;
    assert_equals(flag_approver(&filter, EVENT_PRCTL, value), APPROVED, "value 126 should be approved");

    // values sharing the low 5 bits of an approved value must not match
    value = 30;
    assert_equals(flag_approver(&filter, EVENT_PRCTL, value), DISCARDED, "value 30 should be discarded");

    value = 35;
    assert_equals(flag_approver(&filter, EVENT_PRCTL, value), DISCARDED, "value 35 should be discarded");

    value = 63;
    assert_equals(flag_approver(&filter, EVENT_PRCTL, value), DISCARDED, "value 63 should be discarded");

    filter.is_set = 0;
    value = 62;
    assert_equals(flag_approver(&filter, EVENT_PRCTL, value), DISCARDED, "unset filter should discard");

    return 1;
}

SEC("test/zero_init_syscall_state_discarders")
int test_zero_init_syscall_state_discarders() {
    struct syscall_cache_t syscall = {
        .type = EVENT_SETXATTR,
    };

    assert_equals(syscall.state, APPROVED, "zero-initialized syscall state should be APPROVED");
    assert_equals(get_resolver_flags(&syscall, 1), RESOLVER_FLAG_APPLY_DISCARDERS, "zero-initialized syscall should apply discarders");

    syscall.resolver.ret = DENTRY_DISCARDED;
    apply_dentry_resolution_outcome(&syscall, EVENT_SETXATTR);
    assert_equals(syscall.state, DISCARDED, "DENTRY_DISCARDED should discard APPROVED syscall");

    struct syscall_cache_t accepted_syscall = {
        .type = EVENT_OPEN,
        .state = ACCEPTED,
    };
    assert_equals(get_resolver_flags(&accepted_syscall, 1), 0, "ACCEPTED syscall should not apply discarders");

    return 1;
}

#endif
