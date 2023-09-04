#ifndef _HOOKS_UTIME_H_
#define _HOOKS_UTIME_H_

#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) trace__sys_utimes() {
    struct policy_t policy = fetch_policy(EVENT_UTIME);
    if (is_discarded_by_process(policy.mode, EVENT_UTIME)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_UTIME,
        .policy = policy,
    };

    cache_syscall(&syscall);

    return 0;
}

// On old kernels, we have sys_utime and compat_sys_utime.
// On new kernels, we have _x64_sys_utime32, __ia32_sys_utime32, __x64_sys_utime, __ia32_sys_utime
HOOK_SYSCALL_COMPAT_ENTRY0(utime) {
    return trace__sys_utimes();
}

HOOK_SYSCALL_ENTRY0(utime32) {
    return trace__sys_utimes();
}

HOOK_SYSCALL_COMPAT_TIME_ENTRY0(utimes) {
    return trace__sys_utimes();
}

HOOK_SYSCALL_COMPAT_TIME_ENTRY0(utimensat) {
    return trace__sys_utimes();
}

HOOK_SYSCALL_COMPAT_TIME_ENTRY0(futimesat) {
    return trace__sys_utimes();
}

int __attribute__((always_inline)) sys_utimes_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_UTIME);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct utimes_event_t event = {
        .syscall.retval = retval,
        .atime = syscall->setattr.atime,
        .mtime = syscall->setattr.mtime,
        .file = syscall->setattr.file,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    // dentry resolution in setattr.h

    send_event(ctx, EVENT_UTIME, event);

    return 0;
}

HOOK_SYSCALL_COMPAT_EXIT(utime) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_utimes_ret(ctx, retval);
}

HOOK_SYSCALL_EXIT(utime32) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_utimes_ret(ctx, retval);
}

HOOK_SYSCALL_COMPAT_TIME_EXIT(utimes) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_utimes_ret(ctx, retval);
}

HOOK_SYSCALL_COMPAT_TIME_EXIT(utimensat) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_utimes_ret(ctx, retval);
}

HOOK_SYSCALL_COMPAT_TIME_EXIT(futimesat) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_utimes_ret(ctx, retval);
}

SEC("tracepoint/handle_sys_utimes_exit")
int tracepoint_handle_sys_utimes_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_utimes_ret(args, args->ret);
}

#endif
