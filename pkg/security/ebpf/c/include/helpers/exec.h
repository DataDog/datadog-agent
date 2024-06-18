#ifndef _HELPERS_EXEC_H
#define _HELPERS_EXEC_H

#include "constants/offsets/filesystem.h"
#include "constants/fentry_macro.h"

#include "process.h"

int __attribute__((always_inline)) handle_exec_event(ctx_t *ctx, struct syscall_cache_t *syscall, struct file *file, struct path *path, struct inode *inode) {
    if (syscall->exec.flags & EXEC_FLAGS_IS_PARSED) {
        return 0;
    }
    syscall->exec.flags |= EXEC_FLAGS_IS_PARSED;

    syscall->exec.dentry = get_file_dentry(file);

    // set mount_id to 0 is this is a fileless exec, meaning that the vfs type is tmpfs and that is an internal mount
    u32 mount_id = is_tmpfs(syscall->exec.dentry) && get_path_mount_flags(path) & MNT_INTERNAL ? 0 : get_path_mount_id(path);

    syscall->exec.file.path_key.ino = get_inode_ino(inode);
    syscall->exec.file.path_key.mount_id = mount_id;
    syscall->exec.file.path_key.path_id = get_path_id(mount_id, 0);

    inc_mount_ref(mount_id);

    // resolve dentry
    syscall->resolver.key = syscall->exec.file.path_key;
    syscall->resolver.dentry = syscall->exec.dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE_OR_FENTRY);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_current_or_impersonated_exec_syscall();

    return 0;
}

bool __attribute__((always_inline)) is_empty_string_array(const char **array) {
    if (!array) {
        return true;
    }

    char *str = (char *)NULL;
    int n = bpf_probe_read(&str, sizeof(str), array);
    if (n != 0 || !str) {
        return true;
    }

    char c = '\0';
    n = bpf_probe_read(&c, sizeof(c), str);
    if (n != 0 || c == '\0') {
        return true;
    }

    return false;
}

#endif
