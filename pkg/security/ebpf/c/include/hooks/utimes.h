#ifndef _HOOKS_UTIME_H_
#define _HOOKS_UTIME_H_

#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) trace__sys_utimes(const char *filename) {
    struct policy_t policy = fetch_policy(EVENT_UTIME);
    struct syscall_cache_t syscall = {
        .type = EVENT_UTIME,
        .policy = policy,
    };

    collect_syscall_ctx(&syscall, SYSCALL_CTX_ARG_STR(0), (void *)filename, NULL, NULL);
    cache_syscall(&syscall);

    return 0;
}

// On old kernels, we have sys_utime and compat_sys_utime.
// On new kernels, we have _x64_sys_utime32, __ia32_sys_utime32, __x64_sys_utime, __ia32_sys_utime
HOOK_SYSCALL_COMPAT_ENTRY1(utime, const char *, filename) {
    return trace__sys_utimes(filename);
}

HOOK_SYSCALL_ENTRY1(utime32, const char *, filename) {
    return trace__sys_utimes(filename);
}

HOOK_SYSCALL_COMPAT_TIME_ENTRY1(utimes, const char *, filename) {
    return trace__sys_utimes(filename);
}

HOOK_SYSCALL_COMPAT_TIME_ENTRY2(utimensat, int, dirfd, const char *, filename) {
    return trace__sys_utimes(filename);
}

HOOK_SYSCALL_COMPAT_TIME_ENTRY2(futimesat, int, dirfd, const char *, filename) {
    return trace__sys_utimes(filename);
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
        .syscall_ctx.id = syscall->ctx_id,
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
