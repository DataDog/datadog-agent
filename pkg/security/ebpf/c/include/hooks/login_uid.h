#ifndef _HOOKS_LOGIN_UID_H_
#define _HOOKS_LOGIN_UID_H_

#include "helpers/span_fill.h"
#include "helpers/syscalls.h"

HOOK_ENTRY("audit_set_loginuid")
int hook_audit_set_loginuid(ctx_t *ctx) {
    struct syscall_cache_t syscall = {
        .type = EVENT_LOGIN_UID_WRITE,
        .login_uid = {
            .auid = (u32)CTX_PARM1(ctx),
        },
    };

    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

HOOK_EXIT("audit_set_loginuid")
int rethook_audit_set_loginuid(ctx_t *ctx) {
    int retval = CTX_PARMRET(ctx);
    if (retval < 0) {
        return 0;
    }

    struct syscall_cache_t *syscall = pop_syscall(EVENT_LOGIN_UID_WRITE);
    if (!syscall) {
        return 0;
    }

    // update pid_entry
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    struct pid_cache_t *pid_entry = (struct pid_cache_t *)bpf_map_lookup_elem(&pid_cache, &pid);
    if (!pid_entry) {
        return 0;
    }
    bpf_probe_read(&pid_entry->credentials.auid, sizeof(pid_entry->credentials.auid), &syscall->login_uid.auid);
    pid_entry->credentials.is_auid_set = 1;

    // send event to sync userspace caches
    struct login_uid_write_event_t *event = SPAN_FILL_EVENT(struct login_uid_write_event_t, EVENT_LOGIN_UID_WRITE);
    if (!event) {
        return 0;
    }
    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_cgroup_context(entry, &event->cgroup);

    event->auid = pid_entry->credentials.auid;
    bpf_tail_call_compat(ctx, &span_fill_progs, 0);
    return 0;
}

#endif
