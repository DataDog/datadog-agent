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

int __attribute__((always_inline)) trace__sys_setxattr(char *xattr_name, u64 type) {
    struct syscall_cache_t syscall = {
        .type = type,
        .setxattr = {
            .name = xattr_name,
        }
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE(setxattr) {
    char *name;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&name, sizeof(name), &PT_REGS_PARM2(ctx));
#else
    name = (char *) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_setxattr(name, EVENT_SETXATTR);
}

SYSCALL_KPROBE(lsetxattr) {
    char *name;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&name, sizeof(name), &PT_REGS_PARM2(ctx));
#else
    name = (char *) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_setxattr(name, EVENT_SETXATTR);
}

SYSCALL_KPROBE(fsetxattr) {
    char *name;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&name, sizeof(name), &PT_REGS_PARM2(ctx));
#else
    name = (char *) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_setxattr(name, EVENT_SETXATTR);
}

SYSCALL_KPROBE(removexattr) {
    char *name;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&name, sizeof(name), &PT_REGS_PARM2(ctx));
#else
    name = (char *) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_setxattr(name, EVENT_REMOVEXATTR);
}

SYSCALL_KPROBE(lremovexattr) {
    char *name;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&name, sizeof(name), &PT_REGS_PARM2(ctx));
#else
    name = (char *) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_setxattr(name, EVENT_REMOVEXATTR);
}

SYSCALL_KPROBE(fremovexattr) {
    char *name;
#if USE_SYSCALL_WRAPPER
    ctx = (struct pt_regs *) PT_REGS_PARM1(ctx);
    bpf_probe_read(&name, sizeof(name), &PT_REGS_PARM2(ctx));
#else
    name = (char *) PT_REGS_PARM2(ctx);
#endif
    return trace__sys_setxattr(name, EVENT_REMOVEXATTR);
}

int __attribute__((always_inline)) trace__vfs_setxattr(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    if (syscall->type == EVENT_SETXATTR || syscall->type == EVENT_REMOVEXATTR) {
        if (syscall->setxattr.dentry)
            return 0;
        syscall->setxattr.dentry = (struct dentry *)PT_REGS_PARM1(ctx);
        syscall->setxattr.path_key.ino = get_dentry_ino(syscall->setxattr.dentry);
        // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
        resolve_dentry(syscall->setxattr.dentry, syscall->setxattr.path_key, NULL);
    }

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
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct setxattr_event_t event = {
        .event.type = type,
        .syscall = {
            .retval = retval,
            .timestamp = bpf_ktime_get_ns(),
        },
        .file = {
            .inode = syscall->setxattr.path_key.ino,
            .mount_id = syscall->setxattr.path_key.mount_id,
            .overlay_numlower = get_overlay_numlower(syscall->setxattr.dentry),
        },
    };

    // copy xattr name
    bpf_probe_read_str(&event.name, MAX_XATTR_NAME_LEN, syscall->setxattr.name);

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
