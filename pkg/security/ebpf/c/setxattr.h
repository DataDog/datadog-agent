#ifndef _SETXATTR_H_
#define _SETXATTR_H_

#include "syscalls.h"

struct setxattr_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    char name[MAX_XATTR_NAME_LEN];
};

int __attribute__((always_inline)) trace__sys_setxattr(const char *xattr_name) {
    struct policy_t policy = fetch_policy(EVENT_SETXATTR);
    if (is_discarded_by_process(policy.mode, EVENT_SETXATTR)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_SETXATTR,
        .policy = policy,
        .xattr = {
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
    if (is_discarded_by_process(policy.mode, EVENT_REMOVEXATTR)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_REMOVEXATTR,
        .policy = policy,
        .xattr = {
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
    struct syscall_cache_t *syscall = peek_syscall(event_type);
    if (!syscall) {
        return 0;
    }

    if (syscall->xattr.file.path_key.ino) {
        return 0;
    }

    syscall->xattr.dentry = (struct dentry *)PT_REGS_PARM1(ctx);

    if ((event_type == EVENT_SETXATTR && get_vfs_setxattr_dentry_position() == VFS_ARG_POSITION2) ||
        (event_type == EVENT_REMOVEXATTR && get_vfs_removexattr_dentry_position() == VFS_ARG_POSITION2)) {
        // prevent the verifier from whining
        bpf_probe_read(&syscall->xattr.dentry, sizeof(syscall->xattr.dentry), &syscall->xattr.dentry);
        syscall->xattr.dentry = (struct dentry *) PT_REGS_PARM2(ctx);
    }

    set_file_inode(syscall->xattr.dentry, &syscall->xattr.file, 0);

    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    syscall->resolver.dentry = syscall->xattr.dentry;
    syscall->resolver.key = syscall->xattr.file.path_key;
    syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? event_type : 0;
    syscall->resolver.callback = DR_SETXATTR_CALLBACK_KPROBE_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

int __attribute__((always_inline)) xattr_predicate(u64 type) {
    return type == EVENT_SETXATTR || type == EVENT_REMOVEXATTR;
}

SEC("kprobe/dr_setxattr_callback")
int __attribute__((always_inline)) kprobe_dr_setxattr_callback(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(xattr_predicate);
    if (!syscall) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_SETXATTR);
        return discard_syscall(syscall);
    }

    return 0;
}

SEC("kprobe/vfs_setxattr")
int kprobe_vfs_setxattr(struct pt_regs *ctx) {
    return trace__vfs_setxattr(ctx, EVENT_SETXATTR);
}

SEC("kprobe/vfs_removexattr")
int kprobe_vfs_removexattr(struct pt_regs *ctx) {
    return trace__vfs_setxattr(ctx, EVENT_REMOVEXATTR);
}

int __attribute__((always_inline)) sys_xattr_ret(void *ctx, int retval, u64 event_type) {
    struct syscall_cache_t *syscall = pop_syscall(event_type);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct setxattr_event_t event = {
        .syscall.retval = retval,
        .file = syscall->xattr.file,
    };

    // copy xattr name
    bpf_probe_read_str(&event.name, MAX_XATTR_NAME_LEN, (void*) syscall->xattr.name);

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_file_metadata(syscall->xattr.dentry, &event.file.metadata);
    fill_span_context(&event.span);

    send_event(ctx, event_type, event);

    return 0;
}

int __attribute__((always_inline)) kprobe_sys_setxattr_ret(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return sys_xattr_ret(ctx, retval, EVENT_SETXATTR);
}

SYSCALL_KRETPROBE(setxattr) {
    return kprobe_sys_setxattr_ret(ctx);
}

SYSCALL_KRETPROBE(fsetxattr) {
    return kprobe_sys_setxattr_ret(ctx);
}

SYSCALL_KRETPROBE(lsetxattr) {
    return kprobe_sys_setxattr_ret(ctx);
}

SEC("tracepoint/handle_sys_setxattr_exit")
int tracepoint_handle_sys_setxattr_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_xattr_ret(args, args->ret, EVENT_SETXATTR);
}

int __attribute__((always_inline)) kprobe_sys_removexattr_ret(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return sys_xattr_ret(ctx, retval, EVENT_REMOVEXATTR);
}

SYSCALL_KRETPROBE(removexattr) {
    return kprobe_sys_removexattr_ret(ctx);
}

SYSCALL_KRETPROBE(lremovexattr) {
    return kprobe_sys_removexattr_ret(ctx);
}

SYSCALL_KRETPROBE(fremovexattr) {
    return kprobe_sys_removexattr_ret(ctx);
}

SEC("tracepoint/handle_sys_removexattr_exit")
int tracepoint_handle_sys_removexattr_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_xattr_ret(args, args->ret, EVENT_REMOVEXATTR);
}

#endif
