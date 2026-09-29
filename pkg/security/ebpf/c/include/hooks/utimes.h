#ifndef _HOOKS_UTIME_H_
#define _HOOKS_UTIME_H_

#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/span_fill.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) trace__sys_utimes(void *ctx, const char *filename) {
    if (is_discarded_by_pid() || is_auid_discarder(EVENT_UTIME)) {
        return 0;
    }

    struct policy_t policy = fetch_policy(EVENT_UTIME);
    struct syscall_cache_t syscall = {
        .type = EVENT_UTIME,
        .policy = policy,
    };

    collect_syscall_ctx(&syscall, SYSCALL_CTX_ARG_STR(0), (void *)filename, NULL, NULL);
    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

// On old kernels, we have sys_utime and compat_sys_utime.
// On new kernels, we have _x64_sys_utime32, __ia32_sys_utime32, __x64_sys_utime, __ia32_sys_utime
HOOK_SYSCALL_COMPAT_ENTRY1(utime, const char *, filename) {
    return trace__sys_utimes(ctx, filename);
}

HOOK_SYSCALL_ENTRY1(utime32, const char *, filename) {
    return trace__sys_utimes(ctx, filename);
}

HOOK_SYSCALL_COMPAT_TIME_ENTRY1(utimes, const char *, filename) {
    return trace__sys_utimes(ctx, filename);
}

HOOK_SYSCALL_COMPAT_TIME_ENTRY2(utimensat, int, dirfd, const char *, filename) {
    return trace__sys_utimes(ctx, filename);
}

HOOK_SYSCALL_COMPAT_TIME_ENTRY2(futimesat, int, dirfd, const char *, filename) {
    return trace__sys_utimes(ctx, filename);
}

int __attribute__((always_inline)) sys_utimes_ret_impl(void *ctx, int retval, enum TAIL_CALL_PROG_TYPE prog_type) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_UTIME);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    set_file_layer(syscall->resolver.dentry, &syscall->setattr.file);

    struct utimes_event_t *event = SPAN_FILL_EVENT(struct utimes_event_t, EVENT_UTIME);
    if (!event) {
        return 0;
    }
    event->syscall.retval = retval;
    event->syscall_ctx.id = syscall->ctx_id;
    event->atime = syscall->setattr.atime;
    event->mtime = syscall->setattr.mtime;
    event->file = syscall->setattr.file;

    struct proc_cache_t *entry = fill_process_context(&event->process);
    fill_cgroup_context(entry, &event->cgroup);

    // dentry resolution in setattr.h

    span_fill_tail_call(ctx, prog_type);

    return 0;
}

int __attribute__((always_inline)) sys_utimes_ret(void *ctx, int retval) {
    return sys_utimes_ret_impl(ctx, retval, KPROBE_OR_FENTRY_TYPE);
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

TAIL_CALL_TRACEPOINT_FNC(handle_sys_utimes_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_utimes_ret_impl(args, args->ret, TRACEPOINT_TYPE);
}

#endif
