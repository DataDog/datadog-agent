#ifndef _RMDIR_H_
#define _RMDIR_H_

#include "syscalls.h"

struct rmdir_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
};

SYSCALL_KPROBE0(rmdir) {
    struct syscall_cache_t syscall = {
        .type = SYSCALL_RMDIR,
    };

    cache_syscall(&syscall, EVENT_RMDIR);

    return 0;
}

SEC("kprobe/security_inode_rmdir")
int kprobe__security_inode_rmdir(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(SYSCALL_RMDIR | SYSCALL_UNLINK);
    if (!syscall)
        return 0;

    u64 event_type = 0;
    struct path_key_t key = {};
    struct dentry *dentry = NULL;

    u32 path_id = get_path_id(1);

    switch (syscall->type) {
        case SYSCALL_RMDIR:
            event_type = EVENT_RMDIR;

            // we resolve all the information before the file is actually removed
            dentry = (struct dentry *)PT_REGS_PARM2(ctx);

            // if second pass, ex: overlayfs, just cache the inode that will be used in ret
            if (syscall->rmdir.path_key.ino) {
                syscall->rmdir.real_inode = get_dentry_ino(dentry);
                return 0;
            }

            syscall->rmdir.path_key.ino = get_dentry_ino(dentry);
            syscall->rmdir.overlay_numlower = get_overlay_numlower(dentry);
            syscall->rmdir.path_key.path_id = path_id;

            // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
            key = syscall->rmdir.path_key;

            break;
        case SYSCALL_UNLINK:
            event_type = EVENT_UNLINK;

            // we resolve all the information before the file is actually removed
            dentry = (struct dentry *) PT_REGS_PARM2(ctx);

            // if second pass, ex: overlayfs, just cache the inode that will be used in ret
            if (syscall->unlink.path_key.ino) {
                syscall->unlink.real_inode = get_dentry_ino(dentry);
                return 0;
            }

            syscall->unlink.overlay_numlower = get_overlay_numlower(dentry);
            syscall->unlink.path_key.ino = get_dentry_ino(dentry);
            syscall->unlink.path_key.path_id = path_id;

            // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
            key = syscall->unlink.path_key;

            break;
    }

    if (discarded_by_process(syscall->policy.mode, event_type)) {
        invalidate_inode(ctx, key.mount_id, key.ino, 1);

        return 0;
    }

    if (dentry != NULL) {
        int ret = resolve_dentry(dentry, key, syscall->policy.mode != NO_FILTER ? event_type : 0);
        if (ret == DENTRY_DISCARDED) {
            invalidate_inode(ctx, key.mount_id, key.ino, 1);

            pop_syscall(syscall->type);
        }
    }

    return 0;
}

SYSCALL_KRETPROBE(rmdir) {
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_RMDIR | SYSCALL_UNLINK);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);

    // add an real entry to reach the first dentry with the proper inode
    u64 inode = syscall->rmdir.path_key.ino;
    if (syscall->rmdir.real_inode) {
        inode = syscall->rmdir.real_inode;
        link_dentry_inode(syscall->rmdir.path_key, inode);
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        invalidate_inode(ctx, syscall->rmdir.path_key.mount_id, inode, 0);
        return 0;
    }

    int enabled = is_event_enabled(EVENT_RMDIR);
    if (enabled) {
        struct rmdir_event_t event = {
            .event.type = EVENT_RMDIR,
            .event.timestamp = bpf_ktime_get_ns(),
            .syscall.retval = retval,
            .file = {
                .inode = inode,
                .mount_id = syscall->rmdir.path_key.mount_id,
                .overlay_numlower = syscall->rmdir.overlay_numlower,
                .path_id = syscall->rmdir.path_key.path_id,
            }
        };

        struct proc_cache_t *entry = fill_process_data(&event.process);
        fill_container_data(entry, &event.container);

        send_event(ctx, event);
    }

    invalidate_inode(ctx, syscall->rmdir.path_key.mount_id, inode, !enabled);

    return 0;
}

#endif
