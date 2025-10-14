#ifndef _HOOKS_PRCTL_H_
#define _HOOKS_PRCTL_H_

#include "constants/syscall_macro.h"
#include "helpers/syscalls.h"
#include "helpers/process.h"
#include <linux/prctl.h>

static long __attribute__((always_inline)) trace__sys_prctl(u8 async, int option, void * arg2) {
    if (is_discarded_by_pid()) {
        return 0;
    }
    struct policy_t policy = fetch_policy(EVENT_PRCTL);
    struct syscall_cache_t syscall = {
        .type = EVENT_PRCTL,
        .policy = policy,
        .prctl = {
            .option = option,
        }
    };
    switch (option) {
    case PR_SET_NAME: {
        int n = bpf_probe_read_str(&syscall.prctl.name, MAX_PRCTL_NAME_LEN + 1, arg2);
        if (n > 0) {
            syscall.prctl.name_truncated = (n == (MAX_PRCTL_NAME_LEN + 1)) ? 1 : 0;
            syscall.prctl.name_size_to_send = n - 1;
        } else {
            syscall.prctl.name_size_to_send = 0;
        }
        break;
        }
    }
    cache_syscall(&syscall);
    return 0;
}

static int __attribute__((always_inline)) sys_prctl_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_PRCTL);
    if (!syscall) {
        return 0;
    }
    struct prctl_event_t event = {
        .syscall.retval = retval,
        .event.flags = syscall->async,
        .option = syscall->prctl.option,
        .name_truncated = syscall->prctl.name_truncated,        
    };
    bpf_probe_read(&event.name, MAX_PRCTL_NAME_LEN, &syscall->prctl.name);
    event.sent_size = (syscall->prctl.name_size_to_send >= MAX_PRCTL_NAME_LEN)
        ? MAX_PRCTL_NAME_LEN
        : syscall->prctl.name_size_to_send;
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);
    send_event(ctx, EVENT_PRCTL, event);
    return 0;
}

HOOK_SYSCALL_ENTRY2(prctl, int, option, void *, arg2) {
    return trace__sys_prctl(SYNC_SYSCALL, option, arg2);
}

HOOK_SYSCALL_EXIT(prctl) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_prctl_ret(ctx, retval);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_prctl_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_prctl_ret(args, args->ret);
}

#endif
