#ifndef _HOOKS_PIVOT_ROOT_H_
#define _HOOKS_PIVOT_ROOT_H_

#include "constants/syscall_macro.h"
#include "helpers/syscalls.h"

HOOK_SYSCALL_ENTRY0(pivot_root) {
    struct syscall_cache_t syscall = {
        .type = EVENT_PIVOT_ROOT,
    };

    cache_syscall(&syscall);

    return 0;
}

int __attribute__((always_inline)) sys_pivot_root_ret(void *ctx, int retval) {
    pop_syscall(EVENT_PIVOT_ROOT);
    return 0;
}

HOOK_SYSCALL_EXIT(pivot_root) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_pivot_root_ret(ctx, retval);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_pivot_root_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_pivot_root_ret(args, args->ret);
}

#endif
