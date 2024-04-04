#ifndef _HOOKS_CHMOD_H_
#define _HOOKS_CHMOD_H_

#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) trace__sys_chmod(umode_t mode) {
    struct policy_t policy = fetch_policy(EVENT_CHMOD);
    if (is_discarded_by_process(policy.mode, EVENT_CHMOD)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_CHMOD,
        .policy = policy,
        .setattr = {
            .mode = mode & S_IALLUGO,
        }
    };

    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_ENTRY2(chmod, const char*, filename, umode_t, mode) {
    return trace__sys_chmod(mode);
}

HOOK_SYSCALL_ENTRY2(fchmod, int, fd, umode_t, mode) {
    return trace__sys_chmod(mode);
}

HOOK_SYSCALL_ENTRY3(fchmodat, int, dirfd, const char*, filename, umode_t, mode) {
    return trace__sys_chmod(mode);
}

HOOK_SYSCALL_ENTRY4(fchmodat2, int, dirfd, const char*, filename, umode_t, mode, int, flag) {
    return trace__sys_chmod(mode);
}

int __attribute__((always_inline)) sys_chmod_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_CHMOD);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct chmod_event_t event = {
        .syscall.retval = retval,
        .file = syscall->setattr.file,
        .mode = syscall->setattr.mode,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    // dentry resolution in setattr.h

    send_event(ctx, EVENT_CHMOD, event);

    return 0;
}

HOOK_SYSCALL_EXIT(chmod) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_chmod_ret(ctx, retval);
}

HOOK_SYSCALL_EXIT(fchmod) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_chmod_ret(ctx, retval);
}

HOOK_SYSCALL_EXIT(fchmodat) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_chmod_ret(ctx, retval);
}

HOOK_SYSCALL_EXIT(fchmodat2) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_chmod_ret(ctx, retval);
}

SEC("tracepoint/handle_sys_chmod_exit")
int tracepoint_handle_sys_chmod_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_chmod_ret(args, args->ret);
}

#endif
