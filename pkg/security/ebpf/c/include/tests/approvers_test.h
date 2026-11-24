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

SEC("test/approve_bind_connect_sample")
int test_approve_bind_connect_sample(struct baloum_ctx *ctx) {
    u32 pid = 4242;
    u64 cookie = 42;
    u64 cgroup_id = ctx->arg1;
    u64 excluded_cgroup_id = ctx->arg2;
    u16 port = (u16)ctx->arg3;

    struct pid_cache_t pid_entry = {
        .cookie = cookie,
    };
    bpf_map_update_elem(&pid_cache, &pid, &pid_entry, BPF_ANY);

    struct proc_cache_t *proc_entry = (struct proc_cache_t *)ctx->arg0;
    proc_entry->cgroup.path_key.ino = cgroup_id;
    bpf_map_update_elem(&proc_cache, &cookie, proc_entry, BPF_ANY);

    if (excluded_cgroup_id != 0) {
        u8 excluded = 1;
        bpf_map_update_elem(&excluded_cgroups, &excluded_cgroup_id, &excluded, BPF_ANY);
    }

    struct bind_connect_sample_key_t key = {
        .pid = pid,
        .family = AF_INET,
        .protocol = IPPROTO_TCP,
        .port = port,
    };

    return approve_bind_connect_sample(&connect_samples, EVENT_CONNECT, CONNECT_SAMPLE_LIMITER, 0, 40, &key, NULL);
}

SEC("test/open_approvers_sample")
int test_open_approvers_sample(struct baloum_ctx *ctx) {
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u64 cookie = 42;
    u8 from_terminate_walk = (u8)ctx->arg1;
    u64 cgroup_id = ctx->arg3;
    u64 excluded_cgroup_id = ctx->arg4;

    struct pid_cache_t pid_entry = {
        .cookie = cookie,
    };
    bpf_map_update_elem(&pid_cache, &pid, &pid_entry, BPF_ANY);

    struct proc_cache_t *proc_entry = (struct proc_cache_t *)ctx->arg2;
    proc_entry->cgroup.path_key.ino = cgroup_id;
    bpf_map_update_elem(&proc_cache, &cookie, proc_entry, BPF_ANY);

    if (excluded_cgroup_id != 0) {
        u8 excluded = 1;
        bpf_map_update_elem(&excluded_cgroups, &excluded_cgroup_id, &excluded, BPF_ANY);
    }

    struct path_key_t proc_path = {
        .ino = 111,
        .mount_id = 1,
    };
    bpf_map_update_elem(&pid_path_keys, &pid, &proc_path, BPF_ANY);

    struct syscall_cache_t *syscall = (struct syscall_cache_t *)ctx->arg0;
    syscall->type = EVENT_OPEN;
    syscall->open.file.path_key.ino = 222;
    syscall->open.file.path_key.mount_id = 1;
    syscall->open.from_terminate_walk = from_terminate_walk;

    return open_approvers(syscall);
}

#endif
