#ifndef _BIND_H_
#define _BIND_H_

struct bind_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    u64 addr[2];
    u16 family;
    u16 port;
};

SYSCALL_KPROBE3(bind, int, socket, struct sockaddr*, addr, unsigned int, addr_len) {
    if (!addr) {
        return 0;
    }

    struct policy_t policy = fetch_policy(EVENT_BIND);
    if (is_discarded_by_process(policy.mode, EVENT_BIND)) {
        return 0;
    }

    /* cache the bind and wait to grab the retval to send it */
    struct syscall_cache_t syscall = {
        .type = EVENT_BIND,
    };
    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) sys_bind_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_BIND);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    /* pre-fill the event */
    struct bind_event_t event = {
        .syscall.retval = retval,
        .addr[0] = syscall->bind.addr[0],
        .addr[1] = syscall->bind.addr[1],
        .family = syscall->bind.family,
        .port = syscall->bind.port,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);
    send_event(ctx, EVENT_BIND, event);
    return 0;
}

SYSCALL_KRETPROBE(bind) {
    int retval = PT_REGS_RC(ctx);
    return sys_bind_ret(ctx, retval);
}

SEC("tracepoint/syscalls/sys_exit_bind")
int tracepoint_syscalls_sys_exit_bind(struct tracepoint_syscalls_sys_exit_t *args) {
    return sys_bind_ret(args, args->ret);
}

#endif /* _BIND_H_ */
