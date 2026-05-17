#ifndef _HOOKS_SETSID_H_
#define _HOOKS_SETSID_H_

#include "constants/syscall_macro.h"
#include "helpers/syscalls.h"

static int __attribute__((always_inline)) sys_set_sid_ret(void *ctx, int retval) {
    if (pop_syscall(EVENT_SETSID) == NULL) {
        return 0;
    }
    if (retval < 0) {
        return 0;
    }
    // After a successful setsid(2), the calling task is the new session leader,
    // so its session ID equals its (host-namespace) PID by definition.
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    struct pid_cache_t *pid_entry = (struct pid_cache_t *)bpf_map_lookup_elem(&pid_cache, &pid);
    if (!pid_entry) {
        return 0;
    }
    pid_entry->sid = pid;
    return 0;
}

HOOK_SYSCALL_ENTRY0(setsid) {
    struct syscall_cache_t syscall = {
        .type = EVENT_SETSID,
    };
    cache_syscall(&syscall);
    return 0;
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_setsid_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_set_sid_ret(args, args->ret);
}

HOOK_SYSCALL_EXIT(setsid) {
    int retval = (int)SYSCALL_PARMRET(ctx);
    return sys_set_sid_ret(ctx, retval);
}

#endif
