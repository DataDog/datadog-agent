#ifndef _HOOKS_SIGNAL_H_
#define _HOOKS_SIGNAL_H_

#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"

HOOK_SYSCALL_ENTRY2(kill, int, pid, int, type) {
    struct policy_t policy = fetch_policy(EVENT_SIGNAL);
    if (is_discarded_by_process(policy.mode, EVENT_SIGNAL)) {
        return 0;
    }

    /* TODO: implement the event for pid equal to 0 or -1. */
    if (pid < 1) {
        return 0;
    }

    /* cache the signal and wait to grab the retval to send it */
    struct syscall_cache_t syscall = {
        .type = EVENT_SIGNAL,
        .signal = {
            .pid = 0, // 0 in case the root ns pid resolution failed
            .type = type,
        },
    };
    cache_syscall(&syscall);
    return 0;
}

HOOK_ENTRY("check_kill_permission")
int hook_check_kill_permission(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SIGNAL);
    if (!syscall) {
        return 0;
    }

    struct task_struct *task = (struct task_struct *)CTX_PARM3(ctx);
    if (!task) {
        return 0;
    }

    syscall->signal.pid = get_root_nr_from_task_struct(task);

    return 0;
}

/* hook here to grab the EPERM retval */
HOOK_EXIT("check_kill_permission")
int rethook_check_kill_permission(ctx_t* ctx) {
    int retval = (int)CTX_PARMRET(ctx, 3);

    struct syscall_cache_t *syscall = pop_syscall(EVENT_SIGNAL);
    if (!syscall) {
        return 0;
    }

    /* do not send event for signals with EINVAL error code */
    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    /* constuct and send the event */
    struct signal_event_t event = {
        .syscall.retval = retval,
        .pid = syscall->signal.pid,
        .type = syscall->signal.type,
    };
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);
    send_event(ctx, EVENT_SIGNAL, event);
    return 0;
}

#endif /* _HOOKS_SIGNAL_H_ */
