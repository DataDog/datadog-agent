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
#define MEMFD_DENTRY_PREFIX "memfd:"
#define MEMFD_DENTRY_PREFIX_LEN (sizeof(MEMFD_DENTRY_PREFIX) - 1)

#define MEMFD_DENTRY_NAME_MAX_LEN (MEMFD_DENTRY_PREFIX_LEN + MEMFD_TRACER_PREFIX_LEN + TRACER_MEMFD_SUFFIX_LEN)

struct memfd_key_t {
    u32 pid;
    char suffix[TRACER_MEMFD_SUFFIX_LEN];  // Exactly 8 bytes, no nil terminator
};

static bool __attribute__((always_inline)) matches_tracer_prefix(const char *name) {
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

    char name[MEMFD_TRACER_PREFIX_LEN + TRACER_MEMFD_SUFFIX_LEN] = {0};

    long ret = bpf_probe_read_user(&name, sizeof(name), (void *)uname);
    if (ret < 0) {
        return 0;
    }

    if (!matches_tracer_prefix(name)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_TRACER_MEMFD_CREATE,
    };
#pragma unroll
    for (int i = 0; i < TRACER_MEMFD_SUFFIX_LEN; i++) {
        syscall.tracer_memfd_create.suffix[i] = name[MEMFD_TRACER_PREFIX_LEN + i];
    }
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

    // Create tracking entry with PID and suffix as key, fd as value
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;

    struct memfd_key_t key = {
        .pid = pid,
    };

#pragma unroll
    for (int i = 0; i < TRACER_MEMFD_SUFFIX_LEN; i++) {
        key.suffix[i] = syscall->tracer_memfd_create.suffix[i];
    }

    u32 fd = (u32)retval;
    bpf_map_update_elem(&memfd_tracking, &key, &fd, BPF_ANY);

    return 0;
}

static int __attribute__((always_inline)) handle_memfd_fcntl(ctx_t *ctx) {
    struct file *file = (struct file *)CTX_PARM1(ctx);
    unsigned int cmd = (unsigned int)CTX_PARM2(ctx);
    unsigned int arg = (unsigned int)CTX_PARM3(ctx);

    if ((cmd != F_ADD_SEALS) || !(arg & F_SEAL_WRITE)) {
        return 0;
    }

    struct dentry *dentry = get_file_dentry(file);
    if (!dentry) {
        return 0;
    }

    char dentry_name[MEMFD_DENTRY_NAME_MAX_LEN + 1] = {0};
    get_dentry_name(dentry, dentry_name, sizeof(dentry_name));

    // If the name is too short, it can't be one of ours
    if (dentry_name[MEMFD_DENTRY_NAME_MAX_LEN - 1] == 0) {
        return 0;
    }

    // We don't need to compare the prefix, since it's unlikely that a
    // non-tracer memfd name will exactly match our suffix at the exact position
    // at the exact time that we're creating ours.

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;

    struct memfd_key_t key = {
        .pid = pid,
    };
    #pragma unroll
    for (int i = 0; i < TRACER_MEMFD_SUFFIX_LEN; i++) {
        key.suffix[i] = dentry_name[MEMFD_DENTRY_PREFIX_LEN + MEMFD_TRACER_PREFIX_LEN + i];
    }

    u32 *fd = bpf_map_lookup_elem(&memfd_tracking, &key);
    if (!fd) {
        return 0;
    }

    bpf_map_delete_elem(&memfd_tracking, &key);

    struct tracer_memfd_seal_event_t event = {};
    event.event.type = EVENT_TRACER_MEMFD_SEAL;
    event.syscall.retval = 0;
    event.fd = *fd;

    struct proc_cache_t *entry = fill_process_context(&event.process);
    // We don't call fill_span_context(&event.span) to avoid issues with the
    // verifier on 4.14. We know that we don't need the span context for these
    // internal events.
    fill_cgroup_context(entry, &event.cgroup);

    send_event(ctx, EVENT_TRACER_MEMFD_SEAL, event);

    return 0;
}

HOOK_ENTRY("memfd_fcntl")
int hook_memfd_fcntl(ctx_t *ctx) {
    return handle_memfd_fcntl(ctx);
}

// memfd_fcntl was called shmem_fcntl before v4.16
HOOK_ENTRY("shmem_fcntl")
int hook_shmem_fcntl(ctx_t *ctx) {
    return handle_memfd_fcntl(ctx);
}

#endif
