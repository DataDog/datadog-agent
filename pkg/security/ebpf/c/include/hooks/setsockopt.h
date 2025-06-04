#ifndef _HOOKS_SETSOCKOPT_H_
#define _HOOKS_SETSOCKOPT_H_

#include "constants/syscall_macro.h"
#include "helpers/syscalls.h"

long __attribute__((always_inline)) trace__sys_setsock_opt(u8 async, int socket, int level, int optname) {
    if (is_discarded_by_pid()) {
        return 0;
    }

    struct policy_t policy = fetch_policy(EVENT_SETSOCKOPT);
    struct syscall_cache_t syscall = {
        .type = EVENT_SETSOCKOPT,
        .policy = policy,
        .async = async,
        .setsockopt = {
            .level = level,
            .optname = optname,
        }
    };

    cache_syscall(&syscall);

    return 0;
}

int __attribute__((always_inline)) sys_set_sock_opt_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_SETSOCKOPT);

    if (!syscall) {
        return 0;
    }

    struct setsockopt_event_t event = {
        .syscall.retval = retval,
        .level = syscall->setsockopt.level,
        .optname = syscall->setsockopt.optname,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_SETSOCKOPT, event);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_SETSOCKOPT);
    return 0;
}


HOOK_SYSCALL_ENTRY3(setsockopt, int, socket, int, level, int , optname) {
    return trace__sys_setsock_opt(SYNC_SYSCALL, socket, level, optname);
}

HOOK_SYSCALL_EXIT(setsockopt) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_set_sock_opt_ret(ctx, retval);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_setsockopt_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_set_sock_opt_ret(args, args->ret);
}

#endif
