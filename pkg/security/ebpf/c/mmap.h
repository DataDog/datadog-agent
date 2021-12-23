#ifndef _MMAP_H_
#define _MMAP_H_

struct bpf_map_def SEC("maps/mmap_flags_approvers") mmap_flags_approvers = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) approve_mmap_by_flags(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&mmap_flags_approvers, &key);
    if (flags != NULL && (syscall->mmap.flags & *flags) > 0) {
        return 1;
    }
    return 0;
}

struct bpf_map_def SEC("maps/mmap_protection_approvers") mmap_protection_approvers = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) approve_mmap_by_protection(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&mmap_protection_approvers, &key);
    if (flags != NULL && (syscall->mmap.protection & *flags) > 0) {
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) mmap_approvers(struct syscall_cache_t *syscall) {
    int pass_to_userspace = 0;

    if ((syscall->policy.flags & BASENAME) > 0 && syscall->mmap.dentry != NULL) {
        pass_to_userspace = approve_by_basename(syscall->mmap.dentry, EVENT_MMAP);
    }

    if (!pass_to_userspace && (syscall->policy.flags & FLAGS) > 0) {
        pass_to_userspace = approve_mmap_by_protection(syscall);
        if (!pass_to_userspace) {
            pass_to_userspace = approve_mmap_by_flags(syscall);
        }
    }

    return pass_to_userspace;
}

struct mmap_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct file_t file;
    u64 addr;
    u32 len;
    int protection;
    int flags;
    u32 padding;
};

SYSCALL_KPROBE4(mmap, void *, addr, size_t, len, int, protection, int, flags) {
    struct policy_t policy = fetch_policy(EVENT_MMAP);
    if (is_discarded_by_process(policy.mode, EVENT_MMAP)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_MMAP,
        .mmap = {
            .addr = (u64)addr,
            .len = (u32)len,
            .protection = protection,
            .flags = flags,
        }
    };

    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) sys_mmap_ret(void *ctx, int retval, u64 addr) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_MMAP);
    if (!syscall)
        return 0;

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
       return 0;
    }

    if (filter_syscall(syscall, mmap_approvers)) {
        return discard_syscall(syscall);
    }

    struct mmap_event_t event = {
        .syscall.retval = retval,
        .file = syscall->mmap.file,
        .addr = addr,
        .len = syscall->mmap.len,
        .protection = syscall->mmap.protection,
        .flags = syscall->mmap.flags,
    };

    if (syscall->mmap.dentry != NULL) {
        fill_file_metadata(syscall->mmap.dentry, &event.file.metadata);
    }
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_MMAP, event);
    return 0;
}

SYSCALL_KRETPROBE(mmap) {
    return sys_mmap_ret(ctx, (int)PT_REGS_RC(ctx), (u64)PT_REGS_RC(ctx));
}

SEC("tracepoint/syscalls/sys_exit_mmap")
int tracepoint_syscalls_sys_exit_mmap(struct tracepoint_syscalls_sys_exit_t *args) {
    return sys_mmap_ret(args, (int)args->ret, (u64)args->ret);
}

SEC("kretprobe/fget")
int kretprobe_fget(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MMAP);
    if (!syscall)
        return 0;

    struct file *f = (struct file*) PT_REGS_RC(ctx);
    syscall->mmap.dentry = get_file_dentry(f);
    set_file_inode(syscall->mmap.dentry, &syscall->mmap.file, 0);
    syscall->mmap.file.path_key.mount_id = get_file_mount_id(f);

    syscall->resolver.key = syscall->mmap.file.path_key;
    syscall->resolver.dentry = syscall->mmap.dentry;
    syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? EVENT_MMAP : 0;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

#endif
