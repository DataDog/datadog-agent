#ifndef _HOOKS_PRCTL_H_
#define _HOOKS_PRCTL_H_

#include "constants/syscall_macro.h"
#include "helpers/syscalls.h"
#include "helpers/process.h"
long __attribute__((always_inline)) trace__sys_prctl(u8 async, int option) {
    if (is_discarded_by_pid()) {
        return 0;
    }
    struct policy_t policy = fetch_policy(EVENT_PRCTL);
    struct syscall_cache_t syscall = {
        .type = EVENT_PRCTL,
        .policy = policy,
        .async = async,
        .prctl = {
            .option = option,
        }
    };
    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) sys_prctl_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_PRCTL);
    if (!syscall) {
        return 0;
    }
    struct prctl_event_t event = {
        .option = syscall->prctl.option,
    };
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);
    send_event(ctx, EVENT_PRCTL, event);    
    return 0;
}

HOOK_SYSCALL_ENTRY1(prctl, int, option) {
    return trace__sys_prctl(SYNC_SYSCALL, option);
}

HOOK_SYSCALL_EXIT(prctl) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_prctl_ret(ctx, retval);
}
#endif
