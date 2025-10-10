#ifndef _HOOKS_MEMFD_H_
#define _HOOKS_MEMFD_H_

#include "constants/custom.h"
#include "constants/syscall_macro.h"
#include "constants/fentry_macro.h"
#include "helpers/process.h"
#include "helpers/syscalls.h"
#include <linux/fcntl.h>
#include <uapi/linux/memfd.h>

#define MEMFD_TRACER_PREFIX "datadog-tracer-info-"
#define MEMFD_TRACER_PREFIX_LEN (sizeof(MEMFD_TRACER_PREFIX) - 1)

bool __attribute__((always_inline)) matches_tracer_prefix(const char *name) {
    char prefix[MEMFD_TRACER_PREFIX_LEN] = MEMFD_TRACER_PREFIX;

#pragma unroll
    for (int i = 0; i < MEMFD_TRACER_PREFIX_LEN; i++) {
        if (name[i] != prefix[i]) {
            return false;
        }
        if (name[i] == 0) {
            break;
        }
    }
    return true;
}

HOOK_SYSCALL_ENTRY2(memfd_create, const char *, uname, unsigned int, flags) {
    if (is_discarded_by_pid()) {
        return 0;
    }

    if (!(flags & MFD_ALLOW_SEALING)) {
        return 0;
    }

    char name[MEMFD_TRACER_PREFIX_LEN + 1] = {0};

    bpf_probe_read_user_str(&name, sizeof(name), (void *)uname);
    if (!matches_tracer_prefix(name)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_TRACER_MEMFD_CREATE,
    };
    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_EXIT(memfd_create) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_TRACER_MEMFD_CREATE);
    if (!syscall) {
        return 0;
    }

    int retval = SYSCALL_PARMRET(ctx);
    if (retval < 0) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 fd = (u32)retval;
    bpf_map_update_elem(&memfd_tracking, &pid_tgid, &fd, BPF_ANY);

    return 0;
}

int __attribute__((always_inline)) handle_fcntl_seal(void *ctx, u32 fd, unsigned int cmd, unsigned long arg) {
    if ((cmd != F_ADD_SEALS) || !(arg & F_SEAL_WRITE)) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 *tracked_fd = bpf_map_lookup_elem(&memfd_tracking, &pid_tgid);
    if (!tracked_fd) {
        return 0;
    }
    if (*tracked_fd != fd) {
        return 0;
    }

    bpf_map_delete_elem(&memfd_tracking, &pid_tgid);

    struct tracer_memfd_seal_event_t event = {};
    event.event.type = EVENT_TRACER_MEMFD_SEAL;
    event.syscall.retval = 0;
    event.fd = fd;

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_span_context(&event.span);
    fill_cgroup_context(entry, &event.cgroup);

    send_event(ctx, EVENT_TRACER_MEMFD_SEAL, event);

    return 0;
}

HOOK_SYSCALL_ENTRY3(fcntl, unsigned int, fd, unsigned int, cmd, unsigned long, arg) {
    return handle_fcntl_seal(ctx, fd, cmd, arg);
}

HOOK_SYSCALL_ENTRY3(fcntl64, unsigned int, fd, unsigned int, cmd, unsigned long, arg) {
    return handle_fcntl_seal(ctx, fd, cmd, arg);
}

#endif
