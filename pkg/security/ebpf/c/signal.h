#ifndef _SIGNAL_H_
#define _SIGNAL_H_

struct signal_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    u32 pid;
    u32 type;
};

SYSCALL_KPROBE2(kill, int, pid, int, type) {
    struct policy_t policy = fetch_policy(EVENT_SIGNAL);
    if (is_discarded_by_process(policy.mode, EVENT_SIGNAL)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_SIGNAL,
        .signal = {
            .pid = pid,
            .type = type,
        },
    };

    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) sys_kill_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_SIGNAL);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval))
        return 0;

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

SEC("tracepoint/syscalls/sys_exit_kill")
int tracepoint_syscalls_sys_exit_kill(struct tracepoint_syscalls_sys_exit_t *args) {
    return sys_kill_ret(args, (int)args->ret);
}

SYSCALL_KRETPROBE(kill) {
    return sys_kill_ret(ctx, (int)PT_REGS_RC(ctx));
}

#endif /* _SIGNAL_H_ */
