#ifndef _RENAME_H_
#define _RENAME_H_

#include "syscalls.h"

struct rename_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t old;
    struct file_t new;
    u32 discarder_revision;
    u32 padding;
};

int __attribute__((always_inline)) rename_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->rename.src_dentry, EVENT_RENAME) ||
           basename_approver(syscall, syscall->rename.target_dentry, EVENT_RENAME);
}

int __attribute__((always_inline)) trace__sys_rename() {
    struct syscall_cache_t syscall = {
        .policy = fetch_policy(EVENT_RENAME),
        .type = SYSCALL_RENAME,
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE0(rename) {
    return trace__sys_rename();
}

SYSCALL_KPROBE0(renameat) {
    return trace__sys_rename();
}

SYSCALL_KPROBE0(renameat2) {
    return trace__sys_rename();
}

SEC("kprobe/vfs_rename")
int kprobe__vfs_rename(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(SYSCALL_RENAME);
    if (!syscall)
        return 0;

    // if second pass, ex: overlayfs, just cache the inode that will be used in ret
    if (syscall->rename.target_key.ino) {
        return 0;
    }

    struct dentry *src_dentry = (struct dentry *)PT_REGS_PARM2(ctx);
    struct dentry *target_dentry = (struct dentry *)PT_REGS_PARM4(ctx);

    syscall->rename.src_dentry = src_dentry;
    syscall->rename.target_dentry = target_dentry;

    if (filter_syscall(syscall, rename_approvers)) {
        return mark_as_discarded(syscall);
    }

    // use src_dentry as target inode is currently empty and the target file will
    // have the src inode anyway
    set_path_key_inode(src_dentry, &syscall->rename.target_key, 1);

    syscall->rename.src_overlay_numlower = get_overlay_numlower(syscall->rename.src_dentry);

    // we generate a fake source key as the inode is (can be ?) reused
    syscall->rename.src_key.ino = FAKE_INODE_MSW<<32 | bpf_get_prandom_u32();

    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    resolve_dentry(syscall->rename.src_dentry, syscall->rename.src_key, 0);

    // if destination already exists invalidate
    u64 inode = get_dentry_ino(target_dentry);
    if (inode) {
        invalidate_inode(ctx, syscall->rename.target_key.mount_id, inode, 1);
    }

    // If we are discarded, we still want to invalidate the inode
    if (discarded_by_process(syscall->policy.mode, EVENT_RENAME)) {
        return mark_as_discarded(syscall);
    }

    return 0;
}

int __attribute__((always_inline)) trace__sys_rename_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_RENAME);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    u64 inode = get_dentry_ino(syscall->rename.src_dentry);

    // invalidate inode from src dentry to handle ovl folder
    if (syscall->rename.target_key.ino != inode) {
        invalidate_inode(ctx, syscall->rename.target_key.mount_id, inode, 1);
    }

    // invalidate user face inode, so no need to bump the discarder revision in the event
    invalidate_path_key(ctx, &syscall->rename.target_key, 1);

    if (!syscall->discarded && is_event_enabled(EVENT_RENAME)) {
        struct rename_event_t event = {
            .syscall.retval = retval,
            .old = {
                .inode = syscall->rename.src_key.ino,
                .mount_id = syscall->rename.src_key.mount_id,
                .overlay_numlower = syscall->rename.src_overlay_numlower,
            },
            .new = {
                .inode = syscall->rename.target_key.ino,
                .mount_id = syscall->rename.target_key.mount_id,
                .overlay_numlower = get_overlay_numlower(syscall->rename.src_dentry),
                .path_id = syscall->rename.target_key.path_id,
            },
            .discarder_revision = bump_discarder_revision(syscall->rename.target_key.mount_id),
        };

        struct proc_cache_t *entry = fill_process_context(&event.process);
        fill_container_context(entry, &event.container);

        // for centos7, use src dentry for target resolution as the pointers have been swapped
        resolve_dentry(syscall->rename.src_dentry, syscall->rename.target_key, 0);

        send_event(ctx, EVENT_RENAME, event);
    }

    return 0;
}

SYSCALL_KRETPROBE(rename) {
    return trace__sys_rename_ret(ctx);
}

SYSCALL_KRETPROBE(renameat) {
    return trace__sys_rename_ret(ctx);
}

SYSCALL_KRETPROBE(renameat2) {
    return trace__sys_rename_ret(ctx);
}

#endif
