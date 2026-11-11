#ifndef _SYSCALL_USAGE_TEST_H_
#define _SYSCALL_USAGE_TEST_H_

#include "helpers/syscall_usage.h"
#include "baloum.h"

// differs from the pid of the VM so that the process context fill doesn't read the current task
#define SYSCALL_USAGE_TEST_TGID 4242
#define SYSCALL_USAGE_TEST_COOKIE 42
#define SYSCALL_USAGE_TEST_SYSCALL 157

static __always_inline int syscall_usage_test_has(u8 *bitmap, u32 syscall_id) {
    return (bitmap[syscall_id / 8] & (1 << (syscall_id % 8))) != 0;
}

static __always_inline void syscall_usage_test_set_cookie(u32 tgid, u64 cookie) {
    struct pid_cache_t pid_entry = {
        .cookie = cookie,
    };
    bpf_map_update_elem(&pid_cache, &tgid, &pid_entry, BPF_ANY);
}

SEC("test/syscall_usage_record")
int test_syscall_usage_record() {
    u32 tgid = SYSCALL_USAGE_TEST_TGID;

    record_syscall_usage(tgid, SYSCALL_USAGE_TEST_SYSCALL);
    assert_null(bpf_map_lookup_elem(&syscall_usage, &tgid), "usage recorded without an exec cookie");

    syscall_usage_test_set_cookie(tgid, SYSCALL_USAGE_TEST_COOKIE);
    record_syscall_usage(tgid, SYSCALL_USAGE_TEST_SYSCALL);

    struct syscall_usage_t *usage = bpf_map_lookup_elem(&syscall_usage, &tgid);
    assert_not_null(usage, "usage not recorded");
    assert_equals(usage->cookie, SYSCALL_USAGE_TEST_COOKIE, "wrong exec cookie");
    assert_zero(usage->excluded, "process excluded which should not be");
    assert_not_zero(syscall_usage_test_has(usage->period, SYSCALL_USAGE_TEST_SYSCALL), "syscall missing from the period");
    assert_not_zero(syscall_usage_test_has(usage->seen, SYSCALL_USAGE_TEST_SYSCALL), "syscall missing from the seen syscalls");
    assert_not_zero(usage->new_syscalls, "first use of a syscall not flagged as new");

    assert_equals(send_syscall_usage_event(NULL, tgid, usage, SYSCALL_MONITOR_REASON_PERIOD, SYSCALL_USAGE_EVENT_TICKER_SLOT), 1, "event not sent");

    u32 slot = SYSCALL_USAGE_EVENT_TICKER_SLOT;
    struct syscall_monitor_event_t *event = bpf_map_lookup_elem(&syscall_usage_event, &slot);
    assert_not_null(event, "no event slot");
    assert_equals(event->event_reason, SYSCALL_MONITOR_REASON_PERIOD, "wrong event reason");
    assert_not_zero(syscall_usage_test_has((u8 *)event->syscalls, SYSCALL_USAGE_TEST_SYSCALL), "syscall missing from the event");

    assert_zero(syscall_usage_test_has(usage->period, SYSCALL_USAGE_TEST_SYSCALL), "period not reset by the event");
    assert_not_zero(syscall_usage_test_has(usage->seen, SYSCALL_USAGE_TEST_SYSCALL), "seen syscalls reset by the event");
    assert_zero(usage->new_syscalls, "new syscall flag not reset by the event");

    record_syscall_usage(tgid, SYSCALL_USAGE_TEST_SYSCALL);
    assert_not_zero(syscall_usage_test_has(usage->period, SYSCALL_USAGE_TEST_SYSCALL), "syscall missing from the new period");
    assert_zero(usage->new_syscalls, "known syscall flagged as new");

    // the entry outlived its process: it starts over with the cookie of the new one, without an event
    syscall_usage_test_set_cookie(tgid, SYSCALL_USAGE_TEST_COOKIE + 1);
    assert_zero(send_syscall_usage_event(NULL, tgid, usage, SYSCALL_MONITOR_REASON_PERIOD, SYSCALL_USAGE_EVENT_HOOK_SLOT), "event counted for another process");
    assert_equals(usage->cookie, (SYSCALL_USAGE_TEST_COOKIE + 1), "exec cookie not updated");
    assert_zero(syscall_usage_test_has(usage->period, SYSCALL_USAGE_TEST_SYSCALL), "period kept across processes");
    assert_zero(syscall_usage_test_has(usage->seen, SYSCALL_USAGE_TEST_SYSCALL), "seen syscalls kept across processes");

    slot = SYSCALL_USAGE_EVENT_HOOK_SLOT;
    event = bpf_map_lookup_elem(&syscall_usage_event, &slot);
    assert_not_null(event, "no event slot");
    assert_zero(event->event_reason, "event sent for another process");

    record_syscall_usage(tgid, SYSCALL_USAGE_TEST_SYSCALL);
    flush_syscall_usage(NULL, tgid, SYSCALL_MONITOR_REASON_EXIT);
    assert_equals(event->event_reason, SYSCALL_MONITOR_REASON_EXIT, "wrong event reason");
    assert_not_zero(syscall_usage_test_has((u8 *)event->syscalls, SYSCALL_USAGE_TEST_SYSCALL), "syscall missing from the event");
    assert_null(bpf_map_lookup_elem(&syscall_usage, &tgid), "usage kept after the flush");

    return 1;
}

SEC("test/syscall_usage_excluded")
int test_syscall_usage_excluded() {
    u32 tgid = SYSCALL_USAGE_TEST_TGID;

    // the test process has no cgroup: both the current cgroup and the process cache resolve to 0
    u64 cgroup_id = 0;
    u8 excluded = 1;
    bpf_map_update_elem(&excluded_cgroups, &cgroup_id, &excluded, BPF_ANY);

    syscall_usage_test_set_cookie(tgid, SYSCALL_USAGE_TEST_COOKIE);
    record_syscall_usage(tgid, SYSCALL_USAGE_TEST_SYSCALL);

    struct syscall_usage_t *usage = bpf_map_lookup_elem(&syscall_usage, &tgid);
    assert_not_null(usage, "usage not recorded");
    assert_not_zero(usage->excluded, "process of an excluded cgroup not excluded");
    assert_zero(syscall_usage_test_has(usage->period, SYSCALL_USAGE_TEST_SYSCALL), "syscall recorded for an excluded process");
    assert_zero(usage->new_syscalls, "syscall flagged as new for an excluded process");

    return 1;
}

#endif /* _SYSCALL_USAGE_TEST_H_ */
