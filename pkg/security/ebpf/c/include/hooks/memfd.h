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

struct memfd_tracking_t {
    u32 fd;
    char name[28]; // 32 bytes total with fd
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

    char name[32] = {0};

    bpf_probe_read_user_str(&name, sizeof(name), (void *)uname);
    if (!matches_tracer_prefix(name)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_TRACER_MEMFD_CREATE,
    };
    cache_syscall(&syscall);

    // Store the name for later comparison in memfd_fcntl
    // fd will be stored in exit hook
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct memfd_tracking_t tracking = {0};
    bpf_probe_read_str(&tracking.name, sizeof(tracking.name), name);
    bpf_map_update_elem(&memfd_tracking, &pid_tgid, &tracking, BPF_ANY);

    return 0;
}

HOOK_SYSCALL_EXIT(memfd_create) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_TRACER_MEMFD_CREATE);
    if (!syscall) {
        return 0;
    }

    int retval = SYSCALL_PARMRET(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();

    if (retval < 0) {
        // Clean up tracking entry on failure
        bpf_map_delete_elem(&memfd_tracking, &pid_tgid);
        return 0;
    }

    // Store the fd in the tracking entry
    struct memfd_tracking_t *tracking = bpf_map_lookup_elem(&memfd_tracking, &pid_tgid);
    if (tracking) {
        tracking->fd = (u32)retval;
    }

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

    char dentry_name[32] = {0};
    get_dentry_name(dentry, dentry_name, sizeof(dentry_name));

    // Check if the dentry name starts with "memfd:datadog-tracer-info-"
    // Note: kernel adds "memfd:" prefix to the user-provided name
    char expected_prefix[] = "memfd:datadog-tracer-info-";
    #pragma unroll
    for (int i = 0; i < sizeof(expected_prefix) - 1 && i < sizeof(dentry_name); i++) {
        if (dentry_name[i] != expected_prefix[i]) {
            return 0;
        }
    }

    // Get the tracked info for this process
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct memfd_tracking_t *tracking = bpf_map_lookup_elem(&memfd_tracking, &pid_tgid);
    if (!tracking) {
        return 0;
    }

    bpf_printk("handle_fcntl_seal entry: tracking->name=%s", tracking->name);

    // Verify the tracked name matches (dentry_name has "memfd:" prefix, tracking->name doesn't)
    #pragma unroll
    for (int i = 0; i < 28 - 6; i++) {
        if (tracking->name[i] != dentry_name[i + 6]) {
            if (tracking->name[i] == 0 && dentry_name[i + 6] == 0) {
                break;
            }
            return 0;
        }
        if (tracking->name[i] == 0) {
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
