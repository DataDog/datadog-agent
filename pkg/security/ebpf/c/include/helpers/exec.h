#ifndef _HELPERS_EXEC_H
#define _HELPERS_EXEC_H

#include "constants/offsets/filesystem.h"
#include "constants/fentry_macro.h"

#include "process.h"


#define EXEC_INO_ATTEMPTS 0
#define EXEC_INO_ERR_FROM_INODE 1
#define EXEC_INO_ERR_FROM_PATH 2
#define EXEC_INO_ZERO 3
#define EXEC_INO_POINTER_SHAPED 4

// Kernel addresses on both supported architectures sit in the top half, so an "inode number"
// in that range was never one. Distinguishing it from 0 matters: a failed bpf_probe_read
// zeroes its destination, so 0 and a pointer point at different causes.
#define KERNEL_POINTER_FLOOR 0xffff000000000000ULL

static void __attribute__((always_inline)) record_exec_ino_read(int from_inode, long err, unsigned long ino) {
    u32 slot = EXEC_INO_ATTEMPTS;
    u64 *counter = bpf_map_lookup_elem(&exec_ino_read_stats, &slot);
    if (counter != NULL) {
        __sync_fetch_and_add(counter, 1);
    }

    if (err != 0) {
        slot = from_inode ? EXEC_INO_ERR_FROM_INODE : EXEC_INO_ERR_FROM_PATH;
    } else if (ino == 0) {
        slot = EXEC_INO_ZERO;
    } else if (ino >= KERNEL_POINTER_FLOOR) {
        slot = EXEC_INO_POINTER_SHAPED;
    } else {
        return;
    }

    counter = bpf_map_lookup_elem(&exec_ino_read_stats, &slot);
    if (counter != NULL) {
        __sync_fetch_and_add(counter, 1);
    }
}

int __attribute__((always_inline)) handle_exec_event(ctx_t *ctx, struct syscall_cache_t *syscall, struct file *file, struct inode *inode) {
    struct dentry *dentry  = get_file_dentry(file);
    if (syscall->exec.dentry) {
        // handle nlink that needs to be collected in the second pass
        if (dentry) {
            u32 nlink = get_dentry_nlink(dentry);
            if (nlink > syscall->exec.file.metadata.nlink) {
                syscall->exec.file.metadata.nlink = nlink;
            }
            if (is_overlayfs(dentry)) {
                set_overlayfs_nlink(dentry, &syscall->exec.file);
            }
        }
        return 0;
    }
    syscall->exec.dentry = dentry;

    struct path *path = get_file_f_path_addr(file);

    // set mount_id to 0 is this is a fileless exec, meaning that the vfs type is tmpfs and that is an internal mount
    u32 mount_id = is_tmpfs(syscall->exec.dentry) && get_path_mount_flags(path) & MNT_INTERNAL ? 0 : get_path_mount_id(path);

    // Resolve the ino through the checked getters so a failed read is counted rather than
    // silently becoming the path_key. The path->dentry read is still unchecked; a failure
    // there lands in EXEC_INO_ERR_FROM_PATH via the dentry->inode read that follows it.
    long ino_err = 0;
    unsigned long ino = 0;
    if (inode) {
        ino = get_inode_ino_checked(inode, &ino_err);
    } else {
        struct inode *path_inode = get_dentry_inode_checked(get_path_dentry(path), &ino_err);
        if (ino_err == 0) {
            ino = get_inode_ino_checked(path_inode, &ino_err);
        }
    }
    record_exec_ino_read(inode != NULL, ino_err, ino);

    syscall->exec.file.path_key.ino = ino;
    syscall->exec.file.path_key.mount_id = mount_id;
    set_file_inode(syscall->exec.dentry, &syscall->exec.file, PATH_ID_INVALIDATE_TYPE_NONE);

    // debug aid: record that we populated the key, on which task, and for which syscall
    // cache entry. send_exec_event() compares this against what it pops.
    u64 stamp_pid_tgid = bpf_get_current_pid_tgid();
    u32 stamp_tgid = stamp_pid_tgid >> 32;
    struct exec_open_stamp_t stamp = {
        .pid_tgid = stamp_pid_tgid,
        .ino = syscall->exec.file.path_key.ino,
        .mount_id = syscall->exec.file.path_key.mount_id,
        .path_id = syscall->exec.file.path_key.path_id,
        .ctx_id = syscall->ctx_id,
        .padding = 0,
    };
    bpf_map_update_elem(&exec_dentry_open_stamp, &stamp_tgid, &stamp, BPF_ANY);

    // resolve dentry
    syscall->resolver.key = syscall->exec.file.path_key;
    syscall->resolver.dentry = syscall->exec.dentry;
    syscall->resolver.event_type = 0;
    syscall->resolver.flags = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, KPROBE_OR_FENTRY_TYPE);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_current_or_impersonated_exec_syscall();

    return 0;
}

#endif
