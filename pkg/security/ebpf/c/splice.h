#ifndef _SPLICE_H_
#define _SPLICE_H_

struct bpf_map_def SEC("maps/splice_entry_flags_approvers") splice_entry_flags_approvers = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
};

int __attribute__((always_inline)) approve_splice_by_entry_flags(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&splice_entry_flags_approvers, &key);
    if (flags != NULL && (syscall->splice.pipe_entry_flag & *flags) > 0) {
        return 1;
    }
    return 0;
}

struct bpf_map_def SEC("maps/splice_exit_flags_approvers") splice_exit_flags_approvers = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
};

int __attribute__((always_inline)) approve_splice_by_exit_flags(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&splice_exit_flags_approvers, &key);
    if (flags != NULL && (syscall->splice.pipe_exit_flag & *flags) > 0) {
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) splice_approvers(struct syscall_cache_t *syscall) {
    int pass_to_userspace = 0;

    if ((syscall->policy.flags & BASENAME) > 0 && syscall->splice.dentry != NULL) {
        pass_to_userspace = approve_by_basename(syscall->splice.dentry, EVENT_SPLICE);
    }

    if (!pass_to_userspace && (syscall->policy.flags & FLAGS) > 0) {
        pass_to_userspace = approve_splice_by_exit_flags(syscall);
        if (!pass_to_userspace) {
            pass_to_userspace = approve_splice_by_entry_flags(syscall);
        }
    }

    return pass_to_userspace;
}

u64 __attribute__((always_inline)) get_pipe_inode_info_bufs_offset(void) {
    u64 pipe_inode_info_bufs_offset;
    LOAD_CONSTANT("pipe_inode_info_bufs_offset", pipe_inode_info_bufs_offset);
    return pipe_inode_info_bufs_offset;
}

struct splice_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;

    struct file_t file;
    u32 pipe_entry_flag;
    u32 pipe_exit_flag;
};

SYSCALL_KPROBE0(splice) {
    struct policy_t policy = fetch_policy(EVENT_SPLICE);
    if (is_discarded_by_process(policy.mode, EVENT_SPLICE)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_SPLICE,
    };

    cache_syscall(&syscall);
    return 0;
}

SEC("kprobe/get_pipe_info")
int kprobe_get_pipe_info(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SPLICE);
    if (!syscall) {
        return 0;
    }

    // resolve the "in" file path
    if (!syscall->splice.file_found) {
        struct file *f = (struct file*) PT_REGS_PARM1(ctx);
        syscall->splice.dentry = get_file_dentry(f);
        set_file_inode(syscall->splice.dentry, &syscall->splice.file, 0);
        syscall->splice.file.path_key.mount_id = get_file_mount_id(f);
    }

    return 0;
}

SEC("kretprobe/get_pipe_info")
int kretprobe_get_pipe_info(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SPLICE);
    if (!syscall) {
        return 0;
    }

    struct pipe_inode_info *info = (struct pipe_inode_info *)PT_REGS_RC(ctx);
    if (info == NULL) {
        // this is not a pipe, so most likely a file, resolve its path now
        syscall->splice.file_found = 1;
        syscall->resolver.key = syscall->splice.file.path_key;
        syscall->resolver.dentry = syscall->splice.dentry;
        syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? EVENT_SPLICE : 0;
        syscall->resolver.iteration = 0;
        syscall->resolver.ret = 0;

        resolve_dentry(ctx, DR_KPROBE);
        return 0;
    }

    bpf_probe_read(&syscall->splice.bufs, sizeof(syscall->splice.bufs), (void *)info + get_pipe_inode_info_bufs_offset());
    if (syscall->splice.bufs != NULL) {
        // copy the entry flag of the pipe
        bpf_probe_read(&syscall->splice.pipe_entry_flag, sizeof(syscall->splice.pipe_entry_flag), &syscall->splice.bufs->flags);
    }
    return 0;
}

int __attribute__((always_inline)) sys_splice_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_SPLICE);
    if (!syscall) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_SPLICE);
        return 0;
    }

    if (syscall->splice.bufs != NULL) {
        // copy the pipe exit flag
        bpf_probe_read(&syscall->splice.pipe_exit_flag, sizeof(syscall->splice.pipe_exit_flag), &syscall->splice.bufs->flags);
    }

    if (filter_syscall(syscall, splice_approvers)) {
        return discard_syscall(syscall);
    }

    struct splice_event_t event = {
        .syscall.retval = retval,
        .event.async = 0,
        .file = syscall->splice.file,
        .pipe_entry_flag = syscall->splice.pipe_entry_flag,
        .pipe_exit_flag = syscall->splice.pipe_exit_flag,
    };
    fill_file_metadata(syscall->splice.dentry, &event.file.metadata);

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_SPLICE, event);

    return 0;
}

SYSCALL_KRETPROBE(splice) {
    return sys_splice_ret(ctx, (int)PT_REGS_RC(ctx));
}

SEC("tracepoint/handle_sys_splice_exit")
int tracepoint_handle_sys_splice_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_splice_ret(args, args->ret);
}

#endif
