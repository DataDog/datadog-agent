#ifndef _HELPERS_EXEC_H
#define _HELPERS_EXEC_H

#include "constants/offsets/filesystem.h"
#include "constants/fentry_macro.h"

#include "process.h"

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

    syscall->exec.file.path_key.ino = inode ? get_inode_ino(inode) : get_path_ino(path);
    syscall->exec.file.path_key.mount_id = mount_id;
    set_file_inode(syscall->exec.dentry, &syscall->exec.file, PATH_ID_INVALIDATE_TYPE_NONE);

    // debug aid: record that we populated the key, on which task, and for which syscall
    // cache entry. send_exec_event() compares this against what it pops.
    u64 stamp_pid_tgid = bpf_get_current_pid_tgid();
    u32 stamp_tgid = stamp_pid_tgid >> 32;
    struct exec_open_stamp_t stamp = {
        .pid_tgid = stamp_pid_tgid,
        .ctx_id = syscall->ctx_id,
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
