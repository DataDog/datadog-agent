#ifndef _HELPERS_EXEC_H
#define _HELPERS_EXEC_H

#include "constants/offsets/filesystem.h"
#include "constants/fentry_macro.h"

#include "process.h"

int __attribute__((always_inline)) fill_exec_context() {
    struct syscall_cache_t *syscall = peek_current_or_impersonated_exec_syscall();
    if (!syscall) {
        return 0;
    }

    // call it here before the memory get replaced
    fill_span_context(&syscall->exec.span_context);

    return 0;
}

int __attribute__((always_inline)) handle_exec_event(ctx_t *ctx, struct syscall_cache_t *syscall, struct file *file, struct path *path, struct inode *inode) {
    if (syscall->exec.is_parsed) {
        return 0;
    }
    syscall->exec.is_parsed = 1;

    syscall->exec.dentry = get_file_dentry(file);

    // set mount_id to 0 is this is a fileless exec, meaning that the vfs type is tmpfs and that is an internal mount
    u32 mount_id = is_tmpfs(syscall->exec.dentry) && get_path_mount_flags(path) & MNT_INTERNAL ? 0 : get_path_mount_id(path);

    syscall->exec.file.dentry_key.ino = get_inode_ino(inode);
    syscall->exec.file.dentry_key.mount_id = mount_id;
    syscall->exec.file.dentry_key.path_id = get_path_id(syscall->exec.file.dentry_key.mount_id, 0);

    inc_mount_ref(mount_id);

    // resolve dentry
    syscall->resolver.key = syscall->exec.file.dentry_key;
    syscall->resolver.dentry = syscall->exec.dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_CALLBACK_EXECUTABLE;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE_OR_FENTRY);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_EXEC);

    return 0;
}

#endif
