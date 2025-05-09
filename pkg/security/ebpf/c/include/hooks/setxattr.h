#ifndef _HOOKS_SETXATTR_H_
#define _HOOKS_SETXATTR_H_

#include "constants/syscall_macro.h"
#include "helpers/events_predicates.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) trace__sys_setxattr(const char *xattr_name, u8 async, u64 pid_tgid) {
    if (is_discarded_by_pid()) {
        return 0;
    }

    struct policy_t policy = fetch_policy(EVENT_SETXATTR);
    struct syscall_cache_t syscall = {
        .type = EVENT_SETXATTR,
        .policy = policy,
        .async = async,
        .xattr = {
            .name = xattr_name,
        }
    };

    if (pid_tgid > 0) {
        syscall.xattr.pid_tgid = pid_tgid;
    }

    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_ENTRY2(setxattr, const char *, filename, const char *, name) {
    return trace__sys_setxattr(name, 0, 0);
}

HOOK_SYSCALL_ENTRY2(lsetxattr, const char *, filename, const char *, name) {
    return trace__sys_setxattr(name, 0, 0);
}

HOOK_SYSCALL_ENTRY2(fsetxattr, int, fd, const char *, name) {
    return trace__sys_setxattr(name, 0, 0);
}

int __attribute__((always_inline)) trace__sys_removexattr(const char *xattr_name) {
    struct policy_t policy = fetch_policy(EVENT_REMOVEXATTR);
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

HOOK_SYSCALL_ENTRY2(removexattr, const char *, filename, const char *, name) {
    return trace__sys_removexattr(name);
}

HOOK_SYSCALL_ENTRY2(lremovexattr, const char *, filename, const char *, name) {
    return trace__sys_removexattr(name);
}

HOOK_SYSCALL_ENTRY2(fremovexattr, int, fd, const char *, name) {
    return trace__sys_removexattr(name);
}

int __attribute__((always_inline)) trace__vfs_setxattr(ctx_t *ctx, u64 event_type) {
    struct syscall_cache_t *syscall = peek_syscall(event_type);
    if (!syscall) {
        return 0;
    }

    if (syscall->xattr.file.path_key.ino) {
        return 0;
    }

    syscall->xattr.dentry = (struct dentry *)CTX_PARM1(ctx);

    if ((event_type == EVENT_SETXATTR && get_vfs_setxattr_dentry_position() == VFS_ARG_POSITION2) ||
        (event_type == EVENT_REMOVEXATTR && get_vfs_removexattr_dentry_position() == VFS_ARG_POSITION2)) {
        // prevent the verifier from whining
        bpf_probe_read(&syscall->xattr.dentry, sizeof(syscall->xattr.dentry), &syscall->xattr.dentry);
        syscall->xattr.dentry = (struct dentry *)CTX_PARM2(ctx);
        if (syscall->xattr.name == NULL) {
            syscall->xattr.name = (const char *)CTX_PARM3(ctx);
        }
    }

    set_file_inode(syscall->xattr.dentry, &syscall->xattr.file, 0);

    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    syscall->resolver.dentry = syscall->xattr.dentry;
    syscall->resolver.key = syscall->xattr.file.path_key;
    syscall->resolver.discarder_event_type = dentry_resolver_discarder_event_type(syscall);
    syscall->resolver.callback = DR_SETXATTR_CALLBACK_KPROBE_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, KPROBE_OR_FENTRY_TYPE);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(event_type);

    return 0;
}

TAIL_CALL_FNC(dr_setxattr_callback, ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(xattr_predicate);
    if (!syscall) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_SETXATTR);
        pop_syscall(EVENT_SETXATTR);
    }

    return 0;
}

HOOK_ENTRY("vfs_setxattr")
int hook_vfs_setxattr(ctx_t *ctx) {
    return trace__vfs_setxattr(ctx, EVENT_SETXATTR);
}

HOOK_ENTRY("vfs_removexattr")
int hook_vfs_removexattr(ctx_t *ctx) {
    return trace__vfs_setxattr(ctx, EVENT_REMOVEXATTR);
}

int __attribute__((always_inline)) trace_io_fsetxattr(ctx_t *ctx) {
    void *raw_req = (void *)CTX_PARM1(ctx);
    u64 pid_tgid = get_pid_tgid_from_iouring(raw_req);
    return trace__sys_setxattr(NULL, 1, pid_tgid);
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
        .event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0,
        .syscall.retval = retval,
        .file = syscall->xattr.file,
    };

    // copy xattr name
    bpf_probe_read_str(&event.name, MAX_XATTR_NAME_LEN, (void *)syscall->xattr.name);

    struct proc_cache_t *entry;
    if (syscall->xattr.pid_tgid != 0) {
        entry = fill_process_context_with_pid_tgid(&event.process, syscall->xattr.pid_tgid);
    } else {
        entry = fill_process_context(&event.process);
    }

    fill_container_context(entry, &event.container);
    fill_file(syscall->xattr.dentry, &event.file);
    fill_span_context(&event.span);

    send_event(ctx, event_type, event);

    return 0;
}

HOOK_SYSCALL_EXIT(setxattr) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_xattr_ret(ctx, retval, EVENT_SETXATTR);
}

HOOK_SYSCALL_EXIT(fsetxattr) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_xattr_ret(ctx, retval, EVENT_SETXATTR);
}

HOOK_SYSCALL_EXIT(lsetxattr) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_xattr_ret(ctx, retval, EVENT_SETXATTR);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_setxattr_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_xattr_ret(args, args->ret, EVENT_SETXATTR);
}

HOOK_SYSCALL_EXIT(removexattr) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_xattr_ret(ctx, retval, EVENT_REMOVEXATTR);
}

HOOK_SYSCALL_EXIT(lremovexattr) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_xattr_ret(ctx, retval, EVENT_REMOVEXATTR);
}

HOOK_SYSCALL_EXIT(fremovexattr) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_xattr_ret(ctx, retval, EVENT_REMOVEXATTR);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_removexattr_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_xattr_ret(args, args->ret, EVENT_REMOVEXATTR);
}

HOOK_ENTRY("io_fsetxattr")
int hook_io_fsetxattr(ctx_t *ctx) {
    return trace_io_fsetxattr(ctx);
}

HOOK_EXIT("io_fsetxattr")
int rethook_io_fsetxattr(ctx_t *ctx) {
    return sys_xattr_ret(ctx, CTX_PARMRET(ctx), EVENT_SETXATTR);
}

HOOK_ENTRY("io_setxattr")
int hook_io_setxattr(ctx_t *ctx) {
    return trace_io_fsetxattr(ctx);
}

HOOK_EXIT("io_setxattr")
int rethook_io_setxattr(ctx_t *ctx) {
    return sys_xattr_ret(ctx, CTX_PARMRET(ctx), EVENT_SETXATTR);
}

#endif
