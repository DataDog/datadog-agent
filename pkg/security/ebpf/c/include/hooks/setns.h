#ifndef _HOOKS_SETNS_H_
#define _HOOKS_SETNS_H_

#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"
#include "events_definition.h"

static int __attribute__((always_inline)) sys_setns_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_SETNS);
    if (!syscall) {
        return 0;
    }

    // keep the denied attempts, they are as interesting as the successful ones
    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct setns_event_t event = {
        .syscall.retval = retval,
        .fd = syscall->setns.fd,
        .nstype = syscall->setns.nstype,
        .mntns_id = syscall->setns.mntns_id,
        .netns_id = syscall->setns.netns_id,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_SETNS, event);
    return 0;
}

HOOK_SYSCALL_ENTRY2(setns, int, fd, int, nstype) {
    if (is_discarded_by_pid()) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_SETNS,
        .setns = {
            .fd = fd,
            .nstype = nstype,
        }
    };

    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

HOOK_SYSCALL_EXIT(setns) {
    return sys_setns_ret(ctx, (int)SYSCALL_PARMRET(ctx));
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_setns_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_setns_ret(args, args->ret);
}

#endif // _HOOKS_SETNS_H_
