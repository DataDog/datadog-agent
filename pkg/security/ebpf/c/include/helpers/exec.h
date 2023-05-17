#ifndef _HELPERS_EXEC_H
#define _HELPERS_EXEC_H

#include "constants/offsets/filesystem.h"

#include "process.h"

int __attribute__((always_inline)) handle_exec_event(struct pt_regs *ctx, struct syscall_cache_t *syscall, struct file *file, struct path *path, struct inode *inode) {
    if (syscall->exec.is_parsed) {
        return 0;
    }
    syscall->exec.is_parsed = 1;

    syscall->exec.dentry = get_file_dentry(file);

    // set mount_id to 0 is this is a fileless exec, meaning that the vfs type is tmpfs and that is an internal mount
    u32 mount_id = is_tmpfs(syscall->exec.dentry) && get_path_mount_flags(path) & MNT_INTERNAL ? 0 : get_path_mount_id(path);

    syscall->exec.file.path_key.ino = get_inode_ino(inode);
    syscall->exec.file.path_key.mount_id = mount_id;
    syscall->exec.file.path_key.path_id = get_path_id(0);

    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    struct proc_cache_t pc = {
        .entry = {
            .executable = {
                .path_key = {
                    .ino = syscall->exec.file.path_key.ino,
                    .mount_id = mount_id,
                    .path_id = syscall->exec.file.path_key.path_id,
                },
                .flags = syscall->exec.file.flags
            },
            .exec_timestamp = bpf_ktime_get_ns(),
        },
        .container = {},
    };
    fill_file_metadata(syscall->exec.dentry, &pc.entry.executable.metadata);
    bpf_get_current_comm(&pc.entry.comm, sizeof(pc.entry.comm));

    // select the previous cookie entry in cache of the current process
    // (this entry was created by the fork of the current process)
    struct pid_cache_t *fork_entry = (struct pid_cache_t *) bpf_map_lookup_elem(&pid_cache, &tgid);
    if (fork_entry) {
        // Fetch the parent proc cache entry
        u32 parent_cookie = fork_entry->cookie;
        struct proc_cache_t *parent_pc = get_proc_from_cookie(parent_cookie);
        if (parent_pc) {
            // inherit the parent container context
            fill_container_context(parent_pc, &pc.container);
        }
    }

    // Insert new proc cache entry (Note: do not move the order of this block with the previous one, we need to inherit
    // the container ID before saving the entry in proc_cache. Modifying entry after insertion won't work.)
    u32 cookie = bpf_get_prandom_u32();
    bpf_map_update_elem(&proc_cache, &cookie, &pc, BPF_ANY);

    // update pid <-> cookie mapping
    if (fork_entry) {
        fork_entry->cookie = cookie;
    } else {
        struct pid_cache_t new_pid_entry = {
            .cookie = cookie,
        };
        bpf_map_update_elem(&pid_cache, &tgid, &new_pid_entry, BPF_ANY);
    }

    // resolve dentry
    syscall->resolver.key = syscall->exec.file.path_key;
    syscall->resolver.dentry = syscall->exec.dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);

    return 0;
}

#endif
