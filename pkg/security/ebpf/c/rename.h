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
};

int __attribute__((always_inline)) trace__sys_rename() {
    struct syscall_cache_t syscall = {
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

    struct dentry *dentry = (struct dentry *)PT_REGS_PARM2(ctx);

    // if second pass, ex: overlayfs, just cache the inode that will be used in ret
    if (syscall->rename.src_dentry) {
        syscall->rename.real_src_dentry = dentry;
        return 0;
    }

    syscall->rename.src_dentry = dentry;
    syscall->rename.src_overlay_numlower = get_overlay_numlower(syscall->rename.src_dentry);

    // we generate a fake source key as the inode is (can be ?) reused
    syscall->rename.src_key.ino = bpf_get_prandom_u32() << 32 | bpf_get_prandom_u32();

    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    resolve_dentry(syscall->rename.src_dentry, syscall->rename.src_key, NULL);

    return 0;
}

int __attribute__((always_inline)) trace__sys_rename_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_RENAME);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    // Warning: we use the src_dentry twice for compatibility with CentOS. Do not change it :)
    // (the mount id was set by kprobe/mnt_want_write)
    syscall->rename.target_key.ino = get_dentry_ino(syscall->rename.src_dentry);
    if (syscall->rename.real_src_dentry) {
        syscall->rename.target_key.ino = get_dentry_ino(syscall->rename.real_src_dentry);
    }

    struct rename_event_t event = {
        .event.type = EVENT_RENAME,
        .syscall = {
            .retval = retval,
            .timestamp = bpf_ktime_get_ns(),
        },
        .old = {
            .inode = syscall->rename.src_key.ino,
            .mount_id = syscall->rename.src_key.mount_id,
            .overlay_numlower = syscall->rename.src_overlay_numlower,
        },
        .new = {
            .inode = syscall->rename.target_key.ino,
            .mount_id = syscall->rename.target_key.mount_id,
            .overlay_numlower = get_overlay_numlower(syscall->rename.src_dentry),
        }
    };

    struct proc_cache_t *entry = fill_process_data(&event.process);
    fill_container_data(entry, &event.container);

    resolve_dentry(syscall->rename.src_dentry, syscall->rename.target_key, NULL);

    // as old and new have are the same files, only one is needed
    remove_inode_discarders(&event.new);

    send_event(ctx, event);

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
