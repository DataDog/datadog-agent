#ifndef _HOOKS_SIGNAL_H_
#define _HOOKS_SIGNAL_H_

#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"

HOOK_SYSCALL_ENTRY2(kill, int, pid, int, type) {
    if (is_discarded_by_pid()) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_SIGNAL,
        .signal = {
            .type = type,
        },
    };

    if (pid < 1) {
        /*
          in case kill is called with pid 0 or -1 and targets multiple processes, it
          may not go through the kill_permission callpath; but still is valuable to track
        */
        syscall.signal.need_target_resolution = 0;
        syscall.signal.pid = pid;
    } else {
        syscall.signal.need_target_resolution = 1;
        syscall.signal.pid = 0; // it will be resolved later on by check_kill_permission
    }

    cache_syscall(&syscall);
    return 0;
}

HOOK_ENTRY("check_kill_permission")
int hook_check_kill_permission(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SIGNAL);
    if (!syscall || syscall->signal.need_target_resolution == 0) {
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
int rethook_check_kill_permission(ctx_t *ctx) {
    int retval = (int)CTX_PARMRET(ctx);

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
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);
    send_event(ctx, EVENT_SIGNAL, event);
    return 0;
}

#endif /* _HOOKS_SIGNAL_H_ */
