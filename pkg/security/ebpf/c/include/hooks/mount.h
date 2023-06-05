#ifndef _HOOKS_MOUNT_H_
#define _HOOKS_MOUNT_H_

#include "constants/syscall_macro.h"
#include "helpers/events_predicates.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

SEC("kprobe/mnt_want_write")
int kprobe_mnt_want_write(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(mnt_want_write_predicate);
    if (!syscall) {
        return 0;
    }

    struct vfsmount *mnt = (struct vfsmount *)PT_REGS_PARM1(ctx);

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

int __attribute__((always_inline)) trace__mnt_want_write_file(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(mnt_want_write_file_predicate);
    if (!syscall) {
        return 0;
    }

    struct file *file = (struct file *)PT_REGS_PARM1(ctx);
    struct vfsmount *mnt;
    bpf_probe_read(&mnt, sizeof(mnt), &file->f_path.mnt);

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

SEC("kprobe/mnt_want_write_file")
int kprobe_mnt_want_write_file(struct pt_regs *ctx) {
    return trace__mnt_want_write_file(ctx);
}

// mnt_want_write_file_path was used on old kernels (RHEL 7)
SEC("kprobe/mnt_want_write_file_path")
int kprobe_mnt_want_write_file_path(struct pt_regs *ctx) {
    return trace__mnt_want_write_file(ctx);
}

SYSCALL_COMPAT_KPROBE3(mount, const char*, source, const char*, target, const char*, fstype) {
    struct syscall_cache_t syscall = {
        .type = EVENT_MOUNT,
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE1(unshare, unsigned long, flags) {
    struct syscall_cache_t syscall = {
        .type = EVENT_UNSHARE_MNTNS,
        .unshare_mntns = {
            .flags = flags,
        },
    };

    // unshare is only used to propagate mounts created when a mount namespace is copied
    if (!(syscall.unshare_mntns.flags & CLONE_NEWNS)) {
        return 0;
    }

    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/attach_mnt")
int kprobe_attach_mnt(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNSHARE_MNTNS);
    if (!syscall) {
        return 0;
    }

    syscall->unshare_mntns.mnt = (struct mount *)PT_REGS_PARM1(ctx);
    syscall->unshare_mntns.parent = (struct mount *)PT_REGS_PARM2(ctx);
    struct mountpoint *mp = (struct mountpoint *)PT_REGS_PARM3(ctx);
    syscall->unshare_mntns.mp_dentry = get_mountpoint_dentry(mp);

    fill_resolver_mnt(ctx, syscall);
    return 0;
}

SEC("kprobe/__attach_mnt")
int kprobe___attach_mnt(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNSHARE_MNTNS);
    if (!syscall) {
        return 0;
    }

    struct mount *mnt = (struct mount *)PT_REGS_PARM1(ctx);

    // check if mnt has already been processed in case both attach_mnt and __attach_mnt are loaded
    if (syscall->unshare_mntns.mnt == mnt) {
        return 0;
    }

    syscall->unshare_mntns.mnt = mnt;
    syscall->unshare_mntns.parent = (struct mount *)PT_REGS_PARM2(ctx);
    syscall->unshare_mntns.mp_dentry = get_mount_mountpoint_dentry(syscall->unshare_mntns.mnt);

    fill_resolver_mnt(ctx, syscall);
    return 0;
}

SEC("kprobe/mnt_set_mountpoint")
int kprobe_mnt_set_mountpoint(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNSHARE_MNTNS);
    if (!syscall) {
        return 0;
    }

    struct mount *mnt = (struct mount *)PT_REGS_PARM3(ctx);

    // check if mnt has already been processed in case both attach_mnt and __attach_mnt are loaded
    if (syscall->unshare_mntns.mnt == mnt) {
        return 0;
    }

    syscall->unshare_mntns.mnt = mnt;
    syscall->unshare_mntns.parent = (struct mount *)PT_REGS_PARM1(ctx);
    struct mountpoint *mp = (struct mountpoint *)PT_REGS_PARM2(ctx);
    syscall->unshare_mntns.mp_dentry = get_mountpoint_dentry(mp);

    fill_resolver_mnt(ctx, syscall);
    return 0;
}

SEC("kprobe/dr_unshare_mntns_stage_one_callback")
int __attribute__((always_inline)) kprobe_dr_unshare_mntns_stage_one_callback(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNSHARE_MNTNS);
    if (!syscall) {
        return 0;
    }

    struct dentry *mp_dentry = syscall->unshare_mntns.mp_dentry;

    syscall->unshare_mntns.path_key.mount_id = get_mount_mount_id(syscall->unshare_mntns.parent);
    syscall->unshare_mntns.path_key.ino = get_dentry_ino(mp_dentry);

    syscall->resolver.key = syscall->unshare_mntns.path_key;
    syscall->resolver.dentry = mp_dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_UNSHARE_MNTNS_STAGE_TWO_CALLBACK_KPROBE_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

SEC("kprobe/dr_unshare_mntns_stage_two_callback")
int __attribute__((always_inline)) kprobe_dr_unshare_mntns_stage_two_callback(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNSHARE_MNTNS);
    if (!syscall) {
        return 0;
    }

    struct unshare_mntns_event_t event = {
        .mountfields.mount_id = get_mount_mount_id(syscall->unshare_mntns.mnt),
        .mountfields.group_id = get_mount_peer_group_id(syscall->unshare_mntns.mnt),
        .mountfields.device = get_mount_dev(syscall->unshare_mntns.mnt),
        .mountfields.parent_mount_id = syscall->unshare_mntns.path_key.mount_id,
        .mountfields.parent_inode = syscall->unshare_mntns.path_key.ino,
        .mountfields.root_inode = syscall->unshare_mntns.root_key.ino,
        .mountfields.root_mount_id = syscall->unshare_mntns.root_key.mount_id,
        .mountfields.bind_src_mount_id = 0, // do not consider mnt ns copies as bind mounts
    };
    bpf_probe_read_str(&event.mountfields.fstype, FSTYPE_LEN, (void*) syscall->unshare_mntns.fstype);

    if (event.mountfields.mount_id == 0 && event.mountfields.device == 0) {
        return 0;
    }

    send_event(ctx, EVENT_UNSHARE_MNTNS, event);

    return 0;
}

SEC("kprobe/clone_mnt")
int kprobe_clone_mnt(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    if (syscall->mount.bind_src_mnt || syscall->mount.src_mnt) {
        return 0;
    }

    syscall->mount.bind_src_mnt = (struct mount *)PT_REGS_PARM1(ctx);

    syscall->mount.bind_src_key.mount_id = get_mount_mount_id(syscall->mount.bind_src_mnt);
    struct dentry *mount_dentry = get_mount_mountpoint_dentry(syscall->mount.bind_src_mnt);
    syscall->mount.bind_src_key.ino = get_dentry_ino(mount_dentry);

    syscall->resolver.key = syscall->mount.bind_src_key;
    syscall->resolver.dentry = mount_dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

SEC("kprobe/attach_recursive_mnt")
int kprobe_attach_recursive_mnt(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    syscall->mount.src_mnt = (struct mount *)PT_REGS_PARM1(ctx);
    syscall->mount.dest_mnt = (struct mount *)PT_REGS_PARM2(ctx);
    syscall->mount.dest_mountpoint = (struct mountpoint *)PT_REGS_PARM3(ctx);

    // resolve root dentry
    struct dentry *dentry = get_vfsmount_dentry(get_mount_vfsmount(syscall->mount.src_mnt));
    syscall->mount.root_key.mount_id = get_mount_mount_id(syscall->mount.src_mnt);
    syscall->mount.root_key.ino = get_dentry_ino(dentry);

    struct super_block *sb = get_dentry_sb(dentry);
    struct file_system_type *s_type = get_super_block_fs(sb);
    bpf_probe_read(&syscall->mount.fstype, sizeof(syscall->mount.fstype), &s_type->name);

    syscall->resolver.key = syscall->mount.root_key;
    syscall->resolver.dentry = dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

SEC("kprobe/propagate_mnt")
int kprobe_propagate_mnt(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    syscall->mount.dest_mnt = (struct mount *)PT_REGS_PARM1(ctx);
    syscall->mount.dest_mountpoint = (struct mountpoint *)PT_REGS_PARM2(ctx);
    syscall->mount.src_mnt = (struct mount *)PT_REGS_PARM3(ctx);

    // resolve root dentry
    struct dentry *dentry = get_vfsmount_dentry(get_mount_vfsmount(syscall->mount.src_mnt));
    syscall->mount.root_key.mount_id = get_mount_mount_id(syscall->mount.src_mnt);
    syscall->mount.root_key.ino = get_dentry_ino(dentry);

    struct super_block *sb = get_dentry_sb(dentry);
    struct file_system_type *s_type = get_super_block_fs(sb);
    bpf_probe_read(&syscall->mount.fstype, sizeof(syscall->mount.fstype), &s_type->name);

    syscall->resolver.key = syscall->mount.root_key;
    syscall->resolver.dentry = dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = DR_NO_CALLBACK;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

int __attribute__((always_inline)) sys_mount_ret(void *ctx, int retval, int dr_type) {
    if (retval) {
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    struct dentry *dentry = get_mountpoint_dentry(syscall->mount.dest_mountpoint);
    struct path_key_t path_key = {
        .mount_id = get_mount_mount_id(syscall->mount.dest_mnt),
        .ino = get_dentry_ino(dentry),
    };
    syscall->mount.path_key = path_key;

    syscall->resolver.key = path_key;
    syscall->resolver.dentry = dentry;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = dr_type == DR_KPROBE ? DR_MOUNT_CALLBACK_KPROBE_KEY : DR_MOUNT_CALLBACK_TRACEPOINT_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, dr_type);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_MOUNT);
    return 0;
}

SYSCALL_COMPAT_KRETPROBE(mount) {
    int retval = PT_REGS_RC(ctx);
    return sys_mount_ret(ctx, retval, DR_KPROBE);
}

SEC("tracepoint/handle_sys_mount_exit")
int tracepoint_handle_sys_mount_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_mount_ret(args, args->ret, DR_TRACEPOINT);
}

int __attribute__((always_inline)) dr_mount_callback(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_MOUNT);
    if (!syscall) {
        return 0;
    }

    struct mount_event_t event = {
        .syscall.retval = retval,
        .mountfields.mount_id = get_mount_mount_id(syscall->mount.src_mnt),
        .mountfields.group_id = get_mount_peer_group_id(syscall->mount.src_mnt),
        .mountfields.device = get_mount_dev(syscall->mount.src_mnt),
        .mountfields.parent_mount_id = syscall->mount.path_key.mount_id,
        .mountfields.parent_inode = syscall->mount.path_key.ino,
        .mountfields.root_inode = syscall->mount.root_key.ino,
        .mountfields.root_mount_id = syscall->mount.root_key.mount_id,
        .mountfields.bind_src_mount_id = syscall->mount.bind_src_key.mount_id,
    };
    bpf_probe_read_str(&event.mountfields.fstype, FSTYPE_LEN, (void*) syscall->mount.fstype);

    if (event.mountfields.mount_id == 0 && event.mountfields.device == 0) {
        return 0;
    }

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_MOUNT, event);

    return 0;
}

SEC("kprobe/dr_mount_callback")
int __attribute__((always_inline)) kprobe_dr_mount_callback(struct pt_regs *ctx) {
    int ret = PT_REGS_RC(ctx);
    return dr_mount_callback(ctx, ret);
}

SEC("tracepoint/dr_mount_callback")
int __attribute__((always_inline)) tracepoint_dr_mount_callback(struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_mount_callback(args, args->ret);
}

#endif
