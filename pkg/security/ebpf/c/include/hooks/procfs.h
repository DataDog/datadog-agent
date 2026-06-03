#ifndef _HOOKS_PROCFS_H_
#define _HOOKS_PROCFS_H_

#include "constants/custom.h"
#include "constants/offsets/filesystem.h"
#include "helpers/filesystem.h"
#include "helpers/utils.h"

static __attribute__((always_inline)) void cache_file(struct dentry *dentry, u32 mount_id) {
    u64 inode = get_dentry_ino(dentry);
    struct file_t entry = {
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        },
    };

    if (is_overlayfs(dentry)) {
        set_overlayfs_inode(dentry, &entry);
    }

    fill_file(dentry, &entry);

    // cache with the inode as key only as this map is used to capture the mount_id
    // the userspace as to first push an entry so that it limits to eviction caused by other stats from system-probe.
    bpf_map_update_elem(&inode_file, &entry.path_key.ino, &entry, BPF_EXIST);
}

static __attribute__((always_inline)) int handle_stat(void *ctx) {
    if (!is_runtime_request()) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_STAT,
    };
    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

HOOK_SYSCALL_ENTRY0(newfstatat) {
    return handle_stat(ctx);
}

static __attribute__((always_inline)) int handle_ret_stat() {
    if (!is_runtime_request()) {
        return 0;
    }

    pop_syscall(EVENT_STAT);
    return 0;
}

HOOK_SYSCALL_EXIT(newfstatat) {
    return handle_ret_stat();
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_newfstatat_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return handle_ret_stat();
}

// used by both snapshot and process resolver fallback
HOOK_ENTRY("security_inode_getattr")
int hook_security_inode_getattr(ctx_t *ctx) {
    if (!is_runtime_request()) {
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_STAT);
    if (!syscall) {
        return 0;
    }

    if (syscall->stat.in_flight) {
        return 0;
    }
    syscall->stat.in_flight = 1;

    u32 mount_id = 0;
    struct dentry *dentry;

    u64 getattr2 = get_getattr2();

    if (getattr2) {
        struct vfsmount *mnt = (struct vfsmount *)CTX_PARM1(ctx);
        mount_id = get_vfsmount_mount_id(mnt);

        dentry = (struct dentry *)CTX_PARM2(ctx);
    } else {
        struct path *path = (struct path *)CTX_PARM1(ctx);
        mount_id = get_path_mount_id(path);

        dentry = get_path_dentry(path);
    }

    cache_file(dentry, mount_id);

    return 0;
}

#endif
