#ifndef _HOOKS_PTRACE_H_
#define _HOOKS_PTRACE_H_

#include "constants/syscall_macro.h"
#include "helpers/discarders.h"
#include "helpers/syscalls.h"

HOOK_SYSCALL_ENTRY3(ptrace, u32, request, pid_t, pid, void *, addr) {
    struct policy_t policy = fetch_policy(EVENT_PTRACE);
    if (is_discarded_by_process(policy.mode, EVENT_PTRACE)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_PTRACE,
        .ptrace = {
            .request = request,
            .pid = 0, // 0 in case the root ns pid resolution failed
            .addr = (u64)addr,
        }
    };

    cache_syscall(&syscall);
    return 0;
}

HOOK_ENTRY("ptrace_check_attach")
int hook_ptrace_check_attach(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_PTRACE);
    if (!syscall) {
        return 0;
    }

    struct task_struct *child = (struct task_struct *)CTX_PARM1(ctx);
    if (!child) {
        return 0;
    }
    syscall->ptrace.pid = get_root_nr_from_task_struct(child);

    return 0;
}

int __attribute__((always_inline)) sys_ptrace_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_PTRACE);
    if (!syscall) {
        return 0;
    }

    struct ptrace_event_t event = {
        .syscall.retval = retval,
        .request = syscall->ptrace.request,
        .pid = syscall->ptrace.pid,
        .addr = syscall->ptrace.addr,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_PTRACE, event);
    return 0;
}

HOOK_SYSCALL_EXIT(ptrace) {
    return sys_ptrace_ret(ctx, (int)SYSCALL_PARMRET(ctx));
}

SEC("tracepoint/handle_sys_ptrace_exit")
int tracepoint_handle_sys_ptrace_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_ptrace_ret(args, args->ret);
}

#endif
