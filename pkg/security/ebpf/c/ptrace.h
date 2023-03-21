#ifndef _PTRACE_H_
#define _PTRACE_H_

struct ptrace_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    u32 request;
    u32 pid;
    u64 addr;
};

SYSCALL_KPROBE3(ptrace, u32, request, pid_t, pid, void *, addr) {
    struct policy_t policy = fetch_policy(EVENT_PTRACE);
    if (is_discarded_by_process(policy.mode, EVENT_PTRACE)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_PTRACE,
        .ptrace = {
            .request = request,
            .pid = pid,
            .addr = (u64)addr,
        }
    };

    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) sys_ptrace_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_PTRACE);
    if (!syscall) {
        return 0;
    }

    // try to resolve namespaced nr
    u32 pid = get_root_nr(syscall->ptrace.pid);
    if (pid == 0) {
        pid = syscall->ptrace.pid;
    }

    struct ptrace_event_t event = {
        .syscall.retval = retval,
        .request = syscall->ptrace.request,
        .pid = pid,
        .addr = syscall->ptrace.addr,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_PTRACE, event);
    return 0;
}

SYSCALL_KRETPROBE(ptrace) {
    return sys_ptrace_ret(ctx, (int)PT_REGS_RC(ctx));
}

SEC("tracepoint/handle_sys_ptrace_exit")
int tracepoint_handle_sys_ptrace_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_ptrace_ret(args, args->ret);
}

#endif
