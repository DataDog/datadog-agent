#ifndef _MPROTECT_H_
#define _MPROTECT_H_

struct mprotect_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    u64 vm_start;
    u64 vm_end;
    u64 vm_protection;
    u64 req_protection;
};

SYSCALL_KPROBE0(mprotect) {
    struct syscall_cache_t syscall = {
        .type = EVENT_MPROTECT,
    };

    cache_syscall(&syscall);
    return 0;
}

static __attribute__((always_inline)) int is_sensitive_change(u64 vm_protection, u64 req_protection) {
    if ((!(vm_protection & VM_EXEC)) && (req_protection & VM_EXEC)) {
        return 1;
    }
    if ((vm_protection & VM_EXEC) && !(vm_protection & VM_WRITE)
        && ((req_protection & (VM_WRITE|VM_EXEC)) == (VM_WRITE|VM_EXEC))) {
        return 1;
    }
    if (((vm_protection & (VM_WRITE|VM_EXEC)) == (VM_WRITE|VM_EXEC))
        && (req_protection & VM_EXEC) && !(req_protection & VM_WRITE)) {
        return 1;
    }
    return 0;
}

SEC("kprobe/security_file_mprotect")
int kprobe_security_file_mprotect(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MPROTECT);
    if (!syscall)
        return 0;

    // Retrieve vma information
    struct vm_area_struct *vma = (struct vm_area_struct *)PT_REGS_PARM1(ctx);
    bpf_probe_read(&syscall->mprotect.vm_protection, sizeof(syscall->mprotect.vm_protection), &vma->vm_flags);
    bpf_probe_read(&syscall->mprotect.vm_start, sizeof(syscall->mprotect.vm_start), &vma->vm_start);
    bpf_probe_read(&syscall->mprotect.vm_end, sizeof(syscall->mprotect.vm_end), &vma->vm_end);
    syscall->mprotect.req_protection = (u64)PT_REGS_PARM2(ctx);

    // TODO: remove this; for now we only care about sensitive transitions
    if (!is_sensitive_change(syscall->mprotect.vm_protection, syscall->mprotect.req_protection)) {
        pop_syscall(EVENT_MPROTECT);
    }
    return 0;
}

int __attribute__((always_inline)) sys_mprotect_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_MPROTECT);
    if (!syscall)
        return 0;

    struct mprotect_event_t event = {
        .vm_protection = syscall->mprotect.vm_protection,
        .req_protection = syscall->mprotect.req_protection,
        .vm_start = syscall->mprotect.vm_start,
        .vm_end = syscall->mprotect.vm_end,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_MPROTECT, event);
    return 0;
}

SYSCALL_KRETPROBE(mprotect) {
    return sys_mprotect_ret(ctx, (int)PT_REGS_RC(ctx));
}

SEC("tracepoint/syscalls/sys_exit_mprotect")
int tracepoint_syscalls_sys_exit_mprotect(struct tracepoint_syscalls_sys_exit_t *args) {
    return sys_mprotect_ret(args, (int)args->ret);
}

#endif
