#ifndef _HOOKS_MEMFD_H_
#define _HOOKS_MEMFD_H_

#include "constants/custom.h"
#include "constants/syscall_macro.h"
#include "constants/fentry_macro.h"
#include "helpers/process.h"
#include "helpers/syscalls.h"
#include "constants/offsets/filesystem.h"
#include <linux/fcntl.h>
#include <uapi/linux/memfd.h>

#define MEMFD_TRACER_PREFIX "datadog-tracer-info-"
#define MEMFD_TRACER_PREFIX_LEN (sizeof(MEMFD_TRACER_PREFIX) - 1)
#define MEMFD_KERNEL_PREFIX "memfd:"
#define MEMFD_KERNEL_PREFIX_LEN (sizeof(MEMFD_KERNEL_PREFIX) - 1)
#define MEMFD_FULL_PREFIX_LEN (MEMFD_KERNEL_PREFIX_LEN + MEMFD_TRACER_PREFIX_LEN)
#define MEMFD_SUFFIX_MAX_LEN 8  // Maximum length of the suffix after "datadog-tracer-info-"
#define MEMFD_NAME_MAX_LEN (MEMFD_FULL_PREFIX_LEN + MEMFD_SUFFIX_MAX_LEN + 1)  // +1 for null terminator

struct memfd_tracking_t {
    u32 fd;
    char suffix[MEMFD_SUFFIX_MAX_LEN + 1]; // +1 for null terminator
};

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

    char name[MEMFD_NAME_MAX_LEN] = {0};

    bpf_probe_read_user_str(&name, sizeof(name), (void *)uname);
    if (!matches_tracer_prefix(name)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_TRACER_MEMFD_CREATE,
    };
    // Store only the suffix in syscall cache for retrieval in exit hook
    bpf_probe_read_str(&syscall.tracer_memfd_create.suffix, sizeof(syscall.tracer_memfd_create.suffix), name + MEMFD_TRACER_PREFIX_LEN);
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
        // memfd_create failed, no tracking needed
        return 0;
    }

    // Create tracking entry with both fd and suffix from syscall cache
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct memfd_tracking_t tracking = {
        .fd = (u32)retval,
    };
    bpf_probe_read_str(&tracking.suffix, sizeof(tracking.suffix), syscall->tracer_memfd_create.suffix);
    bpf_map_update_elem(&memfd_tracking, &pid_tgid, &tracking, BPF_ANY);

    return 0;
}

HOOK_ENTRY("memfd_fcntl")
int hook_memfd_fcntl(ctx_t *ctx) {
    struct file *file = (struct file *)CTX_PARM1(ctx);
    unsigned int cmd = (unsigned int)CTX_PARM2(ctx);
    unsigned int arg = (unsigned int)CTX_PARM3(ctx);

    if ((cmd != F_ADD_SEALS) || !(arg & F_SEAL_WRITE)) {
        return 0;
    }

    // Get the dentry and read its name
    struct dentry *dentry = get_file_dentry(file);
    if (!dentry) {
        return 0;
    }

    char dentry_name[MEMFD_NAME_MAX_LEN] = {0};
    get_dentry_name(dentry, dentry_name, sizeof(dentry_name));

    // Optimization 1: Reject any names shorter than "memfd:datadog-tracer-info-"
    // Quick length check - if the name is too short, it can't match
    if (dentry_name[MEMFD_FULL_PREFIX_LEN - 1] == 0) {
        return 0;
    }

    // Optimization 2: Compare prefix starting after "memfd:" since all memfd names have this prefix
    // We can skip the "memfd:" part and start comparing at "datadog-tracer-info-"
    char expected_prefix[] = MEMFD_TRACER_PREFIX;
    #pragma unroll
    for (int i = 0; i < MEMFD_TRACER_PREFIX_LEN; i++) {
        if (dentry_name[MEMFD_KERNEL_PREFIX_LEN + i] != expected_prefix[i]) {
            return 0;
        }
    }

    // Get the tracked info for this process
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct memfd_tracking_t *tracking = bpf_map_lookup_elem(&memfd_tracking, &pid_tgid);
    if (!tracking) {
        return 0;
    }

    // Optimization 3 & 4: Compare only the suffix after "memfd:datadog-tracer-info-"
    // tracking->suffix contains only the suffix, dentry_name[MEMFD_FULL_PREFIX_LEN] points to the suffix
    #pragma unroll
    for (int i = 0; i < MEMFD_SUFFIX_MAX_LEN + 1; i++) {
        if (tracking->suffix[i] != dentry_name[MEMFD_FULL_PREFIX_LEN + i]) {
            if (tracking->suffix[i] == 0 && dentry_name[MEMFD_FULL_PREFIX_LEN + i] == 0) {
                break;
            }
            return 0;
        }
        if (tracking->suffix[i] == 0) {
            break;
        }
    }

    u32 fd = tracking->fd;
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

#endif
