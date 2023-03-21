#ifndef _RENAME_H_
#define _RENAME_H_

#include "syscalls.h"

struct rename_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t old;
    struct file_t new;
};

int __attribute__((always_inline)) rename_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->rename.src_dentry, EVENT_RENAME) ||
           basename_approver(syscall, syscall->rename.target_dentry, EVENT_RENAME);
}

int __attribute__((always_inline)) trace__sys_rename(u8 async) {
    struct syscall_cache_t syscall = {
        .policy = fetch_policy(EVENT_RENAME),
        .async = async,
        .type = EVENT_RENAME,
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE0(rename) {
    return trace__sys_rename(SYNC_SYSCALL);
}

SYSCALL_KPROBE0(renameat) {
    return trace__sys_rename(SYNC_SYSCALL);
}

SYSCALL_KPROBE0(renameat2) {
    return trace__sys_rename(SYNC_SYSCALL);
}

SEC("kprobe/do_renameat2")
int kprobe_do_renameat2(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_RENAME);
    if (!syscall) {
        return trace__sys_rename(ASYNC_SYSCALL);
    }
    return 0;
}

SEC("kprobe/vfs_rename")
int kprobe_vfs_rename(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_RENAME);
    if (!syscall) {
        return 0;
    }

    // if second pass, ex: overlayfs, just cache the inode that will be used in ret
    if (syscall->rename.target_file.path_key.ino) {
        return 0;
    }

    struct dentry *src_dentry;
    struct dentry *target_dentry;

    if (get_vfs_rename_input_type() == VFS_RENAME_REGISTER_INPUT) {
        src_dentry = (struct dentry *)PT_REGS_PARM2(ctx);
        target_dentry = (struct dentry *)PT_REGS_PARM4(ctx);
    } else {
        struct renamedata *rename_data = (struct renamedata *)PT_REGS_PARM1(ctx);

        bpf_probe_read(&src_dentry, sizeof(src_dentry), (void *) rename_data + get_vfs_rename_src_dentry_offset());
        bpf_probe_read(&target_dentry, sizeof(target_dentry), (void *) rename_data + get_vfs_rename_target_dentry_offset());
    }

    syscall->rename.src_dentry = src_dentry;
    syscall->rename.target_dentry = target_dentry;

    fill_file_metadata(src_dentry, &syscall->rename.src_file.metadata);
    syscall->rename.target_file.metadata = syscall->rename.src_file.metadata;
    if (is_overlayfs(src_dentry)) {
        syscall->rename.target_file.flags |= UPPER_LAYER;
    }

    // use src_dentry as target inode is currently empty and the target file will
    // have the src inode anyway
    set_file_inode(src_dentry, &syscall->rename.target_file, 1);

    // we generate a fake source key as the inode is (can be ?) reused
    syscall->rename.src_file.path_key.ino = FAKE_INODE_MSW<<32 | bpf_get_prandom_u32();

    // if destination already exists invalidate
    u64 inode = get_dentry_ino(target_dentry);
    if (inode) {
        invalidate_inode(ctx, syscall->rename.target_file.path_key.mount_id, inode, 1);
    }

    // always return after any invalidate_inode call
    if (filter_syscall(syscall, rename_approvers)) {
        return mark_as_discarded(syscall);
    }

    // If we are discarded, we still want to invalidate the inode
    if (is_discarded_by_process(syscall->policy.mode, EVENT_RENAME)) {
        return mark_as_discarded(syscall);
    }

    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    syscall->resolver.dentry = syscall->rename.src_dentry;
    syscall->resolver.key = syscall->rename.src_file.path_key;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

int __attribute__((always_inline)) sys_rename_ret(void *ctx, int retval, int dr_type) {
    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_RENAME);
    if (!syscall) {
        return 0;
    }

    u64 inode = get_dentry_ino(syscall->rename.src_dentry);

    // invalidate inode from src dentry to handle ovl folder
    if (syscall->rename.target_file.path_key.ino != inode && retval >= 0) {
        invalidate_inode(ctx, syscall->rename.target_file.path_key.mount_id, inode, 1);
    }

    int pass_to_userspace = !syscall->discarded && is_event_enabled(EVENT_RENAME);

    // invalidate user space inode, so no need to bump the discarder revision in the event
    if (retval >= 0) {
        invalidate_inode(ctx, syscall->rename.target_file.path_key.mount_id, syscall->rename.target_file.path_key.ino, !pass_to_userspace);

        if (S_ISDIR(syscall->rename.target_file.metadata.mode)) {
            // remove all discarders on the mount point as the rename could invalidate a child discarder in case of a
            // folder rename. For the inode the discarder is invalidated in the ret.
            bump_mount_discarder_revision(syscall->rename.target_file.path_key.mount_id);
        }
    }

    if (pass_to_userspace) {
        // for centos7, use src dentry for target resolution as the pointers have been swapped
        syscall->resolver.key = syscall->rename.target_file.path_key;
        syscall->resolver.dentry = syscall->rename.src_dentry;
        syscall->resolver.discarder_type = 0;
        syscall->resolver.callback = dr_type == DR_KPROBE ? DR_RENAME_CALLBACK_KPROBE_KEY : DR_RENAME_CALLBACK_TRACEPOINT_KEY;
        syscall->resolver.iteration = 0;
        syscall->resolver.ret = 0;

        resolve_dentry(ctx, dr_type);
    }

    // if the tail call failed we need to pop the syscall cache entry
    pop_syscall(EVENT_RENAME);
    return 0;
}

SEC("kretprobe/do_renameat2")
int kretprobe_do_renameat2(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return sys_rename_ret(ctx, retval, DR_KPROBE);
}

int __attribute__((always_inline)) kprobe_sys_rename_ret(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return sys_rename_ret(ctx, retval, DR_KPROBE);
}

SYSCALL_KRETPROBE(rename) {
    return kprobe_sys_rename_ret(ctx);
}

SYSCALL_KRETPROBE(renameat) {
    return kprobe_sys_rename_ret(ctx);
}

SYSCALL_KRETPROBE(renameat2) {
    return kprobe_sys_rename_ret(ctx);
}

SEC("tracepoint/handle_sys_rename_exit")
int tracepoint_handle_sys_rename_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_rename_ret(args, args->ret, DR_TRACEPOINT);
}

int __attribute__((always_inline)) dr_rename_callback(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_RENAME);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct rename_event_t event = {
        .syscall.retval = retval,
        .event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0,
        .old = syscall->rename.src_file,
        .new = syscall->rename.target_file,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_RENAME, event);

    return 0;
}

SEC("kprobe/dr_rename_callback")
int __attribute__((always_inline)) kprobe_dr_rename_callback(struct pt_regs *ctx) {
    int ret = PT_REGS_RC(ctx);
    return dr_rename_callback(ctx, ret);
}

SEC("tracepoint/dr_rename_callback")
int __attribute__((always_inline)) tracepoint_dr_rename_callback(struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_rename_callback(args, args->ret);
}

#endif
