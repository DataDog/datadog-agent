#ifndef _HOOKS_MOUNT_H_
#define _HOOKS_MOUNT_H_

#include "constants/syscall_macro.h"
#include "helpers/events_predicates.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

HOOK_ENTRY("mnt_want_write")
int hook_mnt_want_write(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(mnt_want_write_predicate);
    if (!syscall) {
        return 0;
    }

    struct vfsmount *mnt = (struct vfsmount *)CTX_PARM1(ctx);

    switch (syscall->type) {
    case EVENT_UTIME:
        if (syscall->setattr.file.path_key.mount_id > 0) {
            return 0;
        }
        syscall->setattr.file.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_CHMOD:
        if (syscall->setattr.file.path_key.mount_id > 0) {
            return 0;
        }
        syscall->setattr.file.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_CHOWN:
        if (syscall->setattr.file.path_key.mount_id > 0) {
            return 0;
        }
        syscall->setattr.file.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_RENAME:
        if (syscall->rename.src_file.path_key.mount_id > 0) {
            return 0;
        }
        syscall->rename.src_file.path_key.mount_id = get_vfsmount_mount_id(mnt);
        syscall->rename.target_file.path_key.mount_id = syscall->rename.src_file.path_key.mount_id;
        break;
    case EVENT_RMDIR:
        if (syscall->rmdir.file.path_key.mount_id > 0) {
            return 0;
        }
        syscall->rmdir.file.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_UNLINK:
        if (syscall->unlink.file.path_key.mount_id > 0) {
            return 0;
        }
        syscall->unlink.file.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_SETXATTR:
        if (syscall->xattr.file.path_key.mount_id > 0) {
            return 0;
        }
        syscall->xattr.file.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_REMOVEXATTR:
        if (syscall->xattr.file.path_key.mount_id > 0) {
            return 0;
        }
        syscall->xattr.file.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    }
    return 0;
}

int __attribute__((always_inline)) trace__mnt_want_write_file(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(mnt_want_write_file_predicate);
    if (!syscall) {
        return 0;
    }

    struct file *file = (struct file *)CTX_PARM1(ctx);
    struct vfsmount *mnt;
    bpf_probe_read(&mnt, sizeof(mnt), &get_file_f_path_addr(file)->mnt);

    switch (syscall->type) {
    case EVENT_CHOWN:
        if (syscall->setattr.file.path_key.mount_id > 0) {
            return 0;
        }
        syscall->setattr.file.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_SETXATTR:
        if (syscall->xattr.file.path_key.mount_id > 0) {
            return 0;
        }
        syscall->xattr.file.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_REMOVEXATTR:
        if (syscall->xattr.file.path_key.mount_id > 0) {
            return 0;
        }
        syscall->xattr.file.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    }
    return 0;
}

HOOK_ENTRY("mnt_want_write_file")
int hook_mnt_want_write_file(ctx_t *ctx) {
    return trace__mnt_want_write_file(ctx);
}

// mnt_want_write_file_path was used on old kernels (RHEL 7)
HOOK_ENTRY("mnt_want_write_file_path")
int hook_mnt_want_write_file_path(ctx_t *ctx) {
    return trace__mnt_want_write_file(ctx);
}

HOOK_SYSCALL_COMPAT_ENTRY3(mount, const char *, source, const char *, target, const char *, fstype) {
    struct syscall_cache_t syscall = {
        .type = EVENT_MOUNT,
    };

    collect_syscall_ctx(&syscall, SYSCALL_CTX_ARG_STR(0) | SYSCALL_CTX_ARG_STR(1) | SYSCALL_CTX_ARG_STR(2), (void *)source, (void *)target, (void *)fstype);
    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_ENTRY1(unshare, unsigned long, flags) {
    // unshare is only used to propagate mounts created when a mount namespace is copied
    if (!(flags & CLONE_NEWNS)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_UNSHARE_MNTNS,
    };

    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_EXIT(unshare) {
    pop_syscall(EVENT_UNSHARE_MNTNS);
    return 0;
}

void __attribute__((always_inline)) fill_mount_fields(struct syscall_cache_t *syscall, struct mount_fields_t *mfields) {
    mfields->root_key = syscall->mount.root_key;
    mfields->mountpoint_key = syscall->mount.mountpoint_key;
    mfields->device = syscall->mount.device;
    mfields->bind_src_mount_id = syscall->mount.bind_src_mount_id;
    bpf_probe_read_str(&mfields->fstype, sizeof(mfields->fstype), (void *)syscall->mount.fstype);
}

int __attribute__((always_inline)) send_detached_event(void *ctx, struct syscall_cache_t *syscall) {
    struct mount_event_t event = {
        .syscall.retval = 0,
        .syscall_ctx.id = syscall->ctx_id,
        .source = SOURCE_OPEN_TREE,
        .mountfields.visible = false,
        .mountfields.detached = true,
    };

    if (syscall->type == EVENT_FSMOUNT) {
        event.source = SOURCE_FSMOUNT;
    }

    fill_mount_fields(syscall, &event.mountfields);
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_MOUNT, event);

    return 0;
}

void __attribute__((always_inline)) handle_new_mount(void *ctx, struct syscall_cache_t *syscall, enum TAIL_CALL_PROG_TYPE prog_type, bool detached) {
    // populate the root dentry key
    struct dentry *root_dentry = get_vfsmount_dentry(get_mount_vfsmount(syscall->mount.newmnt));
    syscall->mount.root_key.mount_id = get_mount_mount_id(syscall->mount.newmnt);
    syscall->mount.root_key.ino = get_dentry_ino(root_dentry);
    update_path_id(&syscall->mount.root_key, 0);

    if(!detached) {
        // populate the mountpoint dentry key
        syscall->mount.mountpoint_key.mount_id = get_mount_mount_id(syscall->mount.parent);
        syscall->mount.mountpoint_key.ino = get_dentry_ino(syscall->mount.mountpoint_dentry);
        update_path_id(&syscall->mount.mountpoint_key, 0);
    }

    // populate the device of the new mount
    syscall->mount.device = get_mount_dev(syscall->mount.newmnt);

    // populate the fs type of the new mount
    struct super_block *sb = get_dentry_sb(root_dentry);
    struct file_system_type *s_type = get_super_block_fs(sb);
    bpf_probe_read(&syscall->mount.fstype, sizeof(syscall->mount.fstype), &s_type->name);

    if (syscall->mount.root_key.mount_id == 0 || (!detached && syscall->mount.mountpoint_key.mount_id == 0) || syscall->mount.device == 0) {
        pop_syscall(syscall->type);
        return;
    }

    if(!detached) {
        syscall->resolver.key = syscall->mount.root_key;
        syscall->resolver.dentry = root_dentry;
        syscall->resolver.discarder_event_type = 0;
        syscall->resolver.callback = select_dr_key(prog_type, DR_MOUNT_STAGE_ONE_CALLBACK_KPROBE_KEY, DR_MOUNT_STAGE_ONE_CALLBACK_TRACEPOINT_KEY);
        syscall->resolver.iteration = 0;
        syscall->resolver.ret = 0;

        resolve_dentry(ctx, prog_type);

        // if the tail call fails, we need to pop the syscall cache entry
        pop_syscall(syscall->type);
    } else {
        send_detached_event(ctx, syscall);
    }
}

int __attribute__((always_inline)) dr_mount_stage_one_callback(void *ctx, enum TAIL_CALL_PROG_TYPE prog_type) {
    struct syscall_cache_t *syscall = peek_syscall_with(mountpoint_predicate);
    if (!syscall) {
        return 0;
    }

    syscall->resolver.key = syscall->mount.mountpoint_key;
    syscall->resolver.dentry = syscall->mount.mountpoint_dentry;
    syscall->resolver.discarder_event_type = 0;
    syscall->resolver.callback = select_dr_key(prog_type, DR_MOUNT_STAGE_TWO_CALLBACK_KPROBE_KEY, DR_MOUNT_STAGE_TWO_CALLBACK_TRACEPOINT_KEY);
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, prog_type);
    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(syscall->type);

    return 0;
}

TAIL_CALL_FNC(dr_mount_stage_one_callback, ctx_t *ctx) {
    return dr_mount_stage_one_callback(ctx, KPROBE_OR_FENTRY_TYPE);
}

TAIL_CALL_TRACEPOINT_FNC(dr_mount_stage_one_callback, struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_mount_stage_one_callback(args, TRACEPOINT_TYPE);
}

int __attribute__((always_inline)) dr_mount_stage_two_callback(void *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(mountpoint_predicate);
    if (!syscall) {
        return 0;
    }

    if (syscall->type == EVENT_MOUNT || syscall->type == EVENT_OPEN_TREE) {
        struct mount_event_t event = {
            .syscall.retval = 0,
            .syscall_ctx.id = syscall->ctx_id,
            .source = SOURCE_OPEN_TREE,
            .mountfields.visible = false,
            .mountfields.detached = false,
        };

        fill_mount_fields(syscall, &event.mountfields);
        struct proc_cache_t *entry = fill_process_context(&event.process);
        fill_container_context(entry, &event.container);
        fill_span_context(&event.span);
        if (syscall->type != EVENT_OPEN_TREE) {
            // Only the first mount of a detached copy is detached from the VFS
            // All the other mounts are ultimately attached to the detached mount
            // That's why they aren't detached but are visible
            event.mountfields.visible = true;
            event.source = SOURCE_MOUNT;
            pop_syscall(EVENT_MOUNT);
        }

        send_event(ctx, EVENT_MOUNT, event);

    } else if (syscall->type == EVENT_UNSHARE_MNTNS) {
        struct unshare_mntns_event_t event = { 0 };

        fill_mount_fields(syscall, &event.mountfields);
        send_event(ctx, EVENT_UNSHARE_MNTNS, event);
    }

    return 0;
}

TAIL_CALL_FNC(dr_mount_stage_two_callback, ctx_t *ctx) {
    return dr_mount_stage_two_callback(ctx);
}

TAIL_CALL_TRACEPOINT_FNC(dr_mount_stage_two_callback, struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_mount_stage_two_callback(args);
}

HOOK_ENTRY("attach_mnt")
int hook_attach_mnt(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(unshare_or_open_tree);
    if (!syscall) {
        return 0;
    }

    struct mount *newmnt = (struct mount *)CTX_PARM1(ctx);
    // check if this mount has already been processed
    if (syscall->mount.newmnt == newmnt) {
        return 0;
    }

    syscall->mount.newmnt = newmnt;
    syscall->mount.parent = (struct mount *)CTX_PARM2(ctx);
    struct mountpoint *mp = (struct mountpoint *)CTX_PARM3(ctx);
    syscall->mount.mountpoint_dentry = get_mountpoint_dentry(mp);

    handle_new_mount(ctx, syscall, KPROBE_OR_FENTRY_TYPE, false);

    return 0;
}

HOOK_ENTRY("__attach_mnt")
int hook___attach_mnt(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(unshare_or_open_tree);
    if (!syscall) {
        return 0;
    }

    struct mount *newmnt = (struct mount *)CTX_PARM1(ctx);
    // check if this mount has already been processed by the hook on attach_mnt
    if (syscall->mount.newmnt == newmnt) {
        return 0;
    }

    syscall->mount.newmnt = newmnt;
    syscall->mount.parent = (struct mount *)CTX_PARM2(ctx);
    syscall->mount.mountpoint_dentry = get_mount_mountpoint_dentry(newmnt);

    handle_new_mount(ctx, syscall, KPROBE_OR_FENTRY_TYPE, false);

    return 0;
}

HOOK_ENTRY("mnt_set_mountpoint")
int hook_mnt_set_mountpoint(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNSHARE_MNTNS);
    if (!syscall) {
        return 0;
    }

    struct mount *newmnt = (struct mount *)CTX_PARM3(ctx);
    // check if this mount has already been processed
    if (syscall->mount.newmnt == newmnt) {
        return 0;
    }

    syscall->mount.newmnt = newmnt;
    syscall->mount.parent = (struct mount *)CTX_PARM1(ctx);
    struct mountpoint *mp = (struct mountpoint *)CTX_PARM2(ctx);
    syscall->mount.mountpoint_dentry = get_mountpoint_dentry(mp);

    handle_new_mount(ctx, syscall, KPROBE_OR_FENTRY_TYPE, false);

    return 0;
}

HOOK_ENTRY("clone_mnt")
int hook_clone_mnt(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(mount_or_open_tree);
    if (!syscall) {
        return 0;
    }

    if (syscall->type != EVENT_OPEN_TREE && (syscall->mount.bind_src_mount_id != 0 || syscall->mount.newmnt)) {
        return 0;
    }

    struct mount *bind_src_mnt = (struct mount *)CTX_PARM1(ctx);
    int mount_id = get_mount_mount_id(bind_src_mnt);
    syscall->mount.bind_src_mount_id = mount_id;
    syscall->mount.clone_mnt_ctr++;

    return 0;
}

HOOK_EXIT("clone_mnt")
int rethook_clone_mnt(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_OPEN_TREE);

    if (!syscall) {
        return 0;
    }

    if(syscall->mount.clone_mnt_ctr != 1) {
        return 0;
    }

    struct mount *ret = (struct mount *)CTX_PARMRET(ctx);
    syscall->mount.newmnt = ret;
    handle_new_mount(ctx, syscall, KPROBE_OR_FENTRY_TYPE, true);
    return 0;
}

HOOK_ENTRY("attach_recursive_mnt")
int hook_attach_recursive_mnt(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    struct mount *newmnt = (struct mount *)CTX_PARM1(ctx);
    // check if this mount has already been processed
    if (syscall->mount.newmnt == newmnt) {
        return 0;
    }

    syscall->mount.newmnt = newmnt;
    syscall->mount.parent = (struct mount *)CTX_PARM2(ctx);
    struct mountpoint *mp = (struct mountpoint *)CTX_PARM3(ctx);
    syscall->mount.mountpoint_dentry = get_mountpoint_dentry(mp);

    return 0;
}

HOOK_ENTRY("propagate_mnt")
int hook_propagate_mnt(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    struct mount *newmnt = (struct mount *)CTX_PARM3(ctx);
    // check if this mount has already been processed
    if (syscall->mount.newmnt == newmnt) {
        return 0;
    }

    syscall->mount.newmnt = newmnt;
    syscall->mount.parent = (struct mount *)CTX_PARM1(ctx);
    struct mountpoint *mp = (struct mountpoint *)CTX_PARM2(ctx);
    syscall->mount.mountpoint_dentry = get_mountpoint_dentry(mp);

    return 0;
}

int __attribute__((always_inline)) sys_mount_ret(void *ctx, int retval, enum TAIL_CALL_PROG_TYPE prog_type) {
    if (retval) {
        pop_syscall(EVENT_MOUNT);
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    handle_new_mount(ctx, syscall, prog_type, false);

    return 0;
}

HOOK_SYSCALL_COMPAT_EXIT(mount) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_mount_ret(ctx, retval, KPROBE_OR_FENTRY_TYPE);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_mount_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_mount_ret(args, args->ret, TRACEPOINT_TYPE);
}

HOOK_EXIT("alloc_vfsmnt")
int rethook_alloc_vfsmnt(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_FSMOUNT);
    if (!syscall) {
        return 0;
    }

    struct mount *newmnt = (struct mount *)CTX_PARMRET(ctx);
    syscall->mount.newmnt = newmnt;

    return 0;
}

HOOK_SYSCALL_ENTRY3(open_tree, int, dfd, const char *, filename, unsigned int, flags)
{
    if (!(flags & OPEN_TREE_CLONE)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_OPEN_TREE,
    };
    cache_syscall(&syscall);
    return 0;
}

HOOK_SYSCALL_EXIT(open_tree) {
    pop_syscall(EVENT_OPEN_TREE);
    return 0;
}

HOOK_SYSCALL_ENTRY3(fsmount, int, fs_fd, unsigned int, flags, unsigned int, attr_flags)
{
    struct syscall_cache_t syscall = {
        .type = EVENT_FSMOUNT,
    };

    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_EXIT(fsmount) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_FSMOUNT);
    if (!syscall) {
        // should never happen
        return 0;
    }

    if(syscall->retval >= 0) {
        handle_new_mount(ctx, syscall, KPROBE_OR_FENTRY_TYPE, true);
    }

    return 0;
}

#endif
