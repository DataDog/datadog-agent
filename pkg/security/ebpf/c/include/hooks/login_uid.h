#ifndef _HOOKS_LOGIN_UID_H_
#define _HOOKS_LOGIN_UID_H_

#include "helpers/syscalls.h"

SEC("kprobe/audit_set_loginuid")
int hook_audit_set_loginuid(struct pt_regs *ctx) {
    struct syscall_cache_t syscall = {
        .type = EVENT_LOGIN_UID_WRITE,
        .login_uid = {
            .auid = (u32)PT_REGS_PARM1(ctx),
        },
    };

    cache_syscall(&syscall);
    return 0;
}

SEC("kretprobe/audit_set_loginuid")
int rethook_audit_set_loginuid(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
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
    struct login_uid_write_event_t event = {};
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    event.auid = pid_entry->credentials.auid;
    send_event(ctx, EVENT_LOGIN_UID_WRITE, event);
    return 0;
}

#endif
