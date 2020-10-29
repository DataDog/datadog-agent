#ifndef _SETXATTR_H_
#define _SETXATTR_H_

#include "syscalls.h"

struct setxattr_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    char name[MAX_XATTR_NAME_LEN];
};

int __attribute__((always_inline)) trace__sys_setxattr(const char *xattr_name, u64 type) {
    struct syscall_cache_t syscall = {
        .type = type,
        .setxattr = {
            .name = xattr_name,
        }
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE2(setxattr, const char *, filename, const char *, name) {
    return trace__sys_setxattr(name, SYSCALL_SETXATTR);
}

SYSCALL_KPROBE2(lsetxattr, const char *, filename, const char *, name) {
    return trace__sys_setxattr(name, SYSCALL_SETXATTR);
}

SYSCALL_KPROBE2(fsetxattr, int, fd, const char *, name) {
    return trace__sys_setxattr(name, SYSCALL_SETXATTR);
}

SYSCALL_KPROBE2(removexattr, const char *, filename, const char *, name) {
    return trace__sys_setxattr(name, SYSCALL_REMOVEXATTR);
}

SYSCALL_KPROBE2(lremovexattr, const char *, filename, const char *, name) {
    return trace__sys_setxattr(name, SYSCALL_REMOVEXATTR);
}

SYSCALL_KPROBE2(fremovexattr, int, fd, const char *, name) {
    return trace__sys_setxattr(name, SYSCALL_REMOVEXATTR);
}

int __attribute__((always_inline)) trace__vfs_setxattr(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(SYSCALL_SETXATTR | SYSCALL_REMOVEXATTR);
    if (!syscall)
        return 0;

    struct dentry *dentry = (struct dentry *)PT_REGS_PARM1(ctx);

    // if second pass, ex: overlayfs, just cache the inode that will be used in ret
    if (syscall->setxattr.dentry) {
        syscall->setxattr.real_inode = get_dentry_ino(dentry);
        return 0;
    }

    syscall->setxattr.dentry = dentry;
    syscall->setxattr.path_key.ino = get_dentry_ino(syscall->setxattr.dentry);
    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    resolve_dentry(syscall->setxattr.dentry, syscall->setxattr.path_key, NULL);

    return 0;
}

SEC("kprobe/vfs_setxattr")
int kprobe__vfs_setxattr(struct pt_regs *ctx) {
    return trace__vfs_setxattr(ctx);
}

SEC("kprobe/vfs_removexattr")
int kprobe__vfs_removexattr(struct pt_regs *ctx) {
    return trace__vfs_setxattr(ctx);
}

int __attribute__((always_inline)) trace__sys_setxattr_ret(struct pt_regs *ctx, u64 type) {
    struct syscall_cache_t *syscall = pop_syscall(1 << type);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    // add an real entry to reach the first dentry with the proper inode
    u64 inode = syscall->setxattr.path_key.ino;
    if (syscall->setxattr.real_inode) {
        inode = syscall->setxattr.real_inode;
        link_dentry_inode(syscall->setxattr.path_key, inode);
    }

    struct setxattr_event_t event = {
        .event.type = type,
        .syscall = {
            .retval = retval,
            .timestamp = bpf_ktime_get_ns(),
        },
        .file = {
            .inode = inode,
            .mount_id = syscall->setxattr.path_key.mount_id,
            .overlay_numlower = get_overlay_numlower(syscall->setxattr.dentry),
        },
    };

    // copy xattr name
    bpf_probe_read_str(&event.name, MAX_XATTR_NAME_LEN, (void*) syscall->setxattr.name);

    struct proc_cache_t *entry = fill_process_data(&event.process);
    fill_container_data(entry, &event.container);

    send_event(ctx, event);
    return 0;
}

SYSCALL_KRETPROBE(setxattr) {
    return trace__sys_setxattr_ret(ctx, EVENT_SETXATTR);
}

SYSCALL_KRETPROBE(fsetxattr) {
    return trace__sys_setxattr_ret(ctx, EVENT_SETXATTR);
}

SYSCALL_KRETPROBE(lsetxattr) {
    return trace__sys_setxattr_ret(ctx, EVENT_SETXATTR);
}

SYSCALL_KRETPROBE(removexattr) {
    return trace__sys_setxattr_ret(ctx, EVENT_REMOVEXATTR);
}

SYSCALL_KRETPROBE(lremovexattr) {
    return trace__sys_setxattr_ret(ctx, EVENT_REMOVEXATTR);
}

SYSCALL_KRETPROBE(fremovexattr) {
    return trace__sys_setxattr_ret(ctx, EVENT_REMOVEXATTR);
}

#endif
