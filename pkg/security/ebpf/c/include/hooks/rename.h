#ifndef _HOOKS_RENAME_H_
#define _HOOKS_RENAME_H_

#include "constants/syscall_macro.h"
#include "helpers/approvers.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) trace__sys_rename(u8 async) {
    struct syscall_cache_t syscall = {
        .policy = fetch_policy(EVENT_RENAME),
        .async = async,
        .type = EVENT_RENAME,
    };

    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_ENTRY0(rename) {
    return trace__sys_rename(SYNC_SYSCALL);
}

HOOK_SYSCALL_ENTRY0(renameat) {
    return trace__sys_rename(SYNC_SYSCALL);
}

HOOK_SYSCALL_ENTRY0(renameat2) {
    return trace__sys_rename(SYNC_SYSCALL);
}

HOOK_ENTRY("do_renameat2")
int hook_do_renameat2(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_RENAME);
    if (!syscall) {
        return trace__sys_rename(ASYNC_SYSCALL);
    }
    return 0;
}

HOOK_ENTRY("vfs_rename")
int hook_vfs_rename(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_RENAME);
    if (!syscall) {
        return 0;
    }

    // if second pass, ex: overlayfs, just cache the inode that will be used in ret
    if (syscall->rename.target_file.path_key.ino) {
        return 0;
    }

    struct dentry *src_dentry;
    struct dentry *target_dentry;

    if (get_vfs_rename_input_type() == VFS_RENAME_REGISTER_INPUT) {
        src_dentry = (struct dentry *)CTX_PARM2(ctx);
        target_dentry = (struct dentry *)CTX_PARM4(ctx);
    } else {
        struct renamedata *rename_data = (struct renamedata *)CTX_PARM1(ctx);

        bpf_probe_read(&src_dentry, sizeof(src_dentry), (void *) rename_data + get_vfs_rename_src_dentry_offset());
        bpf_probe_read(&target_dentry, sizeof(target_dentry), (void *) rename_data + get_vfs_rename_target_dentry_offset());
    }

    syscall->rename.src_dentry = src_dentry;
    syscall->rename.target_dentry = target_dentry;

    fill_file(src_dentry, &syscall->rename.src_file);
    syscall->rename.target_file.metadata = syscall->rename.src_file.metadata;
    if (is_overlayfs(src_dentry)) {
        syscall->rename.target_file.flags |= UPPER_LAYER;
    }

    // use src_dentry as target inode is currently empty and the target file will
    // have the src inode anyway
    set_file_inode(src_dentry, &syscall->rename.target_file, 1);

    // we generate a fake source key as the inode is (can be ?) reused
    syscall->rename.src_file.path_key.ino = FAKE_INODE_MSW<<32 | bpf_get_prandom_u32();

    // if destination already exists invalidate
    u64 inode = get_dentry_ino(target_dentry);
    if (inode) {
        expire_inode_discarders(syscall->rename.target_file.path_key.mount_id, inode);
    }

    // always return after any invalidate_inode call
    if (filter_syscall(syscall, rename_approvers)) {
        return mark_as_discarded(syscall);
    }

    // If we are discarded, we still want to invalidate the inode
    if (is_discarded_by_process(syscall->policy.mode, EVENT_RENAME)) {
        return mark_as_discarded(syscall);
    }

    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    syscall->resolver.dentry = syscall->rename.src_dentry;
    syscall->resolver.key = syscall->rename.src_file.path_key;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE_OR_FENTRY);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_RENAME);

    return 0;
}

int __attribute__((always_inline)) sys_rename_ret(void *ctx, int retval, int dr_type) {
    if (IS_UNHANDLED_ERROR(retval)) {
        pop_syscall(EVENT_RENAME);
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_RENAME);
    if (!syscall) {
        return 0;
    }

    u64 inode = get_dentry_ino(syscall->rename.src_dentry);

    // remove discarder inode from src dentry to handle ovl folder
    if (syscall->rename.target_file.path_key.ino != inode && retval >= 0) {
        expire_inode_discarders(syscall->rename.target_file.path_key.mount_id, inode);
    }

    int pass_to_userspace = !syscall->discarded && is_event_enabled(EVENT_RENAME);

    // invalid discarder + path_id
    if (retval >= 0) {
        expire_inode_discarders(syscall->rename.target_file.path_key.mount_id, syscall->rename.target_file.path_key.ino);

        if (S_ISDIR(syscall->rename.target_file.metadata.mode)) {
            // remove all discarders on the mount point as the rename could invalidate a child discarder in case of a
            // folder rename. For the inode the discarder is invalidated in the ret.
            bump_mount_discarder_revision(syscall->rename.target_file.path_key.mount_id);
        }
    }

    if (pass_to_userspace) {
        // for centos7, use src dentry for target resolution as the pointers have been swapped
        syscall->resolver.key = syscall->rename.target_file.path_key;
        syscall->resolver.dentry = syscall->rename.src_dentry;
        syscall->resolver.discarder_type = 0;
        syscall->resolver.callback = select_dr_key(dr_type, DR_RENAME_CALLBACK_KPROBE_KEY, DR_RENAME_CALLBACK_TRACEPOINT_KEY);
        syscall->resolver.iteration = 0;
        syscall->resolver.ret = 0;
        syscall->resolver.sysretval = retval;

        resolve_dentry(ctx, dr_type);
    }

    // if the tail call failed we need to pop the syscall cache entry
    pop_syscall(EVENT_RENAME);
    return 0;
}

HOOK_EXIT("do_renameat2")
int rethook_do_renameat2(ctx_t *ctx) {
    int retval = CTX_PARMRET(ctx, 5);
    return sys_rename_ret(ctx, retval, DR_KPROBE_OR_FENTRY);
}

HOOK_SYSCALL_EXIT(rename) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_rename_ret(ctx, retval, DR_KPROBE_OR_FENTRY);
}

HOOK_SYSCALL_EXIT(renameat) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_rename_ret(ctx, retval, DR_KPROBE_OR_FENTRY);
}

HOOK_SYSCALL_EXIT(renameat2) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_rename_ret(ctx, retval, DR_KPROBE_OR_FENTRY);
}

SEC("tracepoint/handle_sys_rename_exit")
int tracepoint_handle_sys_rename_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_rename_ret(args, args->ret, DR_TRACEPOINT);
}

int __attribute__((always_inline)) dr_rename_callback(void *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_RENAME);
    if (!syscall) {
        return 0;
    }

    s64 retval = syscall->resolver.sysretval;

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct rename_event_t event = {
        .syscall.retval = retval,
        .event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0,
        .old = syscall->rename.src_file,
        .new = syscall->rename.target_file,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_RENAME, event);

    return 0;
}

TAIL_CALL_TARGET("dr_rename_callback")
int tail_call_target_dr_rename_callback(ctx_t *ctx) {
    return dr_rename_callback(ctx);
}

SEC("tracepoint/dr_rename_callback")
int tracepoint_dr_rename_callback(struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_rename_callback(args);
}

#endif
