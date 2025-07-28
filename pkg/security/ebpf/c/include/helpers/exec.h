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
    syscall->exec.file.path_key.path_id = get_path_id(mount_id, 0);

    inc_mount_ref(mount_id);

    // resolve dentry
    syscall->resolver.key = syscall->exec.file.path_key;
    syscall->resolver.dentry = syscall->exec.dentry;
    syscall->resolver.discarder_event_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, KPROBE_OR_FENTRY_TYPE);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_current_or_impersonated_exec_syscall();

    return 0;
}

#endif
