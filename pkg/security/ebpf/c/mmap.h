#ifndef _MMAP_H_
#define _MMAP_H_

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
    // TODO: remove this; for now we only care about memory regions with both VM_WRITE and VM_EXEC activated
    if (!( (protection & (VM_WRITE|VM_EXEC)) == (VM_WRITE|VM_EXEC) || ( (protection & VM_EXEC) == VM_EXEC && (flags & MAP_ANONYMOUS) == 0) )) {
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
