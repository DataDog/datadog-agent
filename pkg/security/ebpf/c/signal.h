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
    BLOCK_SYSCALL_IF_NEEDED_AND_RETURN_DEFAULT_VAL();
    struct policy_t policy = fetch_policy(EVENT_SIGNAL);
    if (is_discarded_by_process(policy.mode, EVENT_SIGNAL)) {
        return 0;
    }

    /* TODO: implement the event for pid equal to 0 or -1. */
    if (pid < 1) {
        return 0;
    }

    /* make a lookup for the target PID in case we are namespaced */
    /* TODO: make a lookup with the key addition of ns or cgroup id */
    u32 root_nr = get_root_nr((u32)pid);
    if (!root_nr) {
        root_nr = pid;
    }

    /* cache the signal and wait to grab the retval to send it */
    struct syscall_cache_t syscall = {
        .type = EVENT_SIGNAL,
        .signal = {
            .pid = root_nr,
            .type = type,
        },
    };
    cache_syscall(&syscall);
    return 0;
}

/* hook here to grab the EPERM retval */
SEC("kretprobe/check_kill_permission")
int kretprobe_check_kill_permission(struct pt_regs* ctx) {
    int retval = (int)PT_REGS_RC(ctx);

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
        .event.async = 0,
        .pid = syscall->signal.pid,
        .type = syscall->signal.type,
    };
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);
    send_event(ctx, EVENT_SIGNAL, event);
    return 0;
}

#endif /* _SIGNAL_H_ */
