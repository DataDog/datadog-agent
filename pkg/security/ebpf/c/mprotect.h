#ifndef _MPROTECT_H_
#define _MPROTECT_H_

struct bpf_map_def SEC("maps/mprotect_vm_protection_approvers") mprotect_vm_protection_approvers = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
};

int __attribute__((always_inline)) approve_mprotect_by_vm_protection(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&mprotect_vm_protection_approvers, &key);
    if (flags != NULL && (syscall->mprotect.vm_protection & *flags) > 0) {
        return 1;
    }
    return 0;
}

struct bpf_map_def SEC("maps/mprotect_req_protection_approvers") mprotect_req_protection_approvers = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
};

int __attribute__((always_inline)) approve_mprotect_by_req_protection(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&mprotect_req_protection_approvers, &key);
    if (flags != NULL && (syscall->mprotect.req_protection & *flags) > 0) {
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) mprotect_approvers(struct syscall_cache_t *syscall) {
    int pass_to_userspace = 0;

    if ((syscall->policy.flags & FLAGS) > 0) {
        int vm_protection_approved = approve_mprotect_by_vm_protection(syscall);
        int req_protection_approved = approve_mprotect_by_req_protection(syscall);
        pass_to_userspace = vm_protection_approved && req_protection_approved;
    }

    return pass_to_userspace;
}

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
    struct policy_t policy = fetch_policy(EVENT_MPROTECT);
    if (is_discarded_by_process(policy.mode, EVENT_MPROTECT)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_MPROTECT,
    };

    cache_syscall(&syscall);
    return 0;
}

SEC("kprobe/security_file_mprotect")
int kprobe_security_file_mprotect(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MPROTECT);
    if (!syscall) {
        return 0;
    }

    // Retrieve vma information
    struct vm_area_struct *vma = (struct vm_area_struct *)PT_REGS_PARM1(ctx);
    bpf_probe_read(&syscall->mprotect.vm_protection, sizeof(syscall->mprotect.vm_protection), &vma->vm_flags);
    bpf_probe_read(&syscall->mprotect.vm_start, sizeof(syscall->mprotect.vm_start), &vma->vm_start);
    bpf_probe_read(&syscall->mprotect.vm_end, sizeof(syscall->mprotect.vm_end), &vma->vm_end);
    syscall->mprotect.req_protection = (u64)PT_REGS_PARM2(ctx);
    return 0;
}

int __attribute__((always_inline)) sys_mprotect_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_MPROTECT);
    if (!syscall) {
        return 0;
    }

    if (filter_syscall(syscall, mprotect_approvers)) {
        return discard_syscall(syscall);
    }

    struct mprotect_event_t event = {
        .event.async = 0,
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

SEC("tracepoint/handle_sys_mprotect_exit")
int tracepoint_handle_sys_mprotect_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_mprotect_ret(args, args->ret);
}

#endif
