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

int __attribute__((always_inline)) trace__sys_setxattr(const char *xattr_name) {
    struct policy_t policy = fetch_policy(EVENT_SETXATTR);
    if (discarded_by_process(policy.mode, EVENT_SETXATTR)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = SYSCALL_SETXATTR,
        .policy = policy,
        .setxattr = {
            .name = xattr_name,
        }
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE2(setxattr, const char *, filename, const char *, name) {
    return trace__sys_setxattr(name);
}

SYSCALL_KPROBE2(lsetxattr, const char *, filename, const char *, name) {
    return trace__sys_setxattr(name);
}

SYSCALL_KPROBE2(fsetxattr, int, fd, const char *, name) {
    return trace__sys_setxattr(name);
}

int __attribute__((always_inline)) trace__sys_removexattr(const char *xattr_name) {
    struct policy_t policy = fetch_policy(EVENT_REMOVEXATTR);
    if (discarded_by_process(policy.mode, EVENT_REMOVEXATTR)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = SYSCALL_REMOVEXATTR,
        .policy = policy,
        .setxattr = {
            .name = xattr_name,
        }
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE2(removexattr, const char *, filename, const char *, name) {
    return trace__sys_removexattr(name);
}

SYSCALL_KPROBE2(lremovexattr, const char *, filename, const char *, name) {
    return trace__sys_removexattr(name);
}

SYSCALL_KPROBE2(fremovexattr, int, fd, const char *, name) {
    return trace__sys_removexattr(name);
}

int __attribute__((always_inline)) trace__vfs_setxattr(struct pt_regs *ctx, u64 event_type) {
    struct syscall_cache_t *syscall = peek_syscall(1 << event_type);
    if (!syscall)
        return 0;

    if (syscall->setxattr.path_key.ino) {
        return 0;
    }

    struct dentry *dentry = (struct dentry *)PT_REGS_PARM1(ctx);
    syscall->setxattr.dentry = dentry;

    set_path_key_inode(syscall->setxattr.dentry, &syscall->setxattr.path_key, 0);

    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    int ret = resolve_dentry(syscall->setxattr.dentry, syscall->setxattr.path_key, syscall->policy.mode != NO_FILTER ? event_type : 0);
    if (ret == DENTRY_DISCARDED) {
        return 0;
    }

    return 0;
}

SEC("kprobe/vfs_setxattr")
int kprobe__vfs_setxattr(struct pt_regs *ctx) {
    return trace__vfs_setxattr(ctx, EVENT_SETXATTR);
}

SEC("kprobe/vfs_removexattr")
int kprobe__vfs_removexattr(struct pt_regs *ctx) {
    return trace__vfs_setxattr(ctx, EVENT_REMOVEXATTR);
}

int __attribute__((always_inline)) trace__sys_setxattr_ret(struct pt_regs *ctx, u64 event_type) {
    struct syscall_cache_t *syscall = pop_syscall(1 << event_type);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct setxattr_event_t event = {
        .syscall.retval = retval,
        .file = {
            .inode = syscall->setxattr.path_key.ino,
            .mount_id = syscall->setxattr.path_key.mount_id,
            .overlay_numlower = get_overlay_numlower(syscall->setxattr.dentry),
            .path_id = syscall->setxattr.path_key.path_id,
        },
    };

    // copy xattr name
    bpf_probe_read_str(&event.name, MAX_XATTR_NAME_LEN, (void*) syscall->setxattr.name);

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);

    send_event(ctx, event_type, event);

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
