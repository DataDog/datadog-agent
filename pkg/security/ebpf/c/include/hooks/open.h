#ifndef _HOOKS_OPEN_H_
#define _HOOKS_OPEN_H_

#include "constants/syscall_macro.h"
#include "constants/fentry_macro.h"
#include "helpers/approvers.h"
#include "helpers/discarders.h"
#include "helpers/filesystem.h"
#include "helpers/exec.h"
#include "helpers/iouring.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) trace__sys_openat2(const char *path, u8 async, int flags, umode_t mode, u64 pid_tgid) {
    if (is_discarded_by_pid()) {
        return 0;
    }

    struct policy_t policy = fetch_policy(EVENT_OPEN);
    struct syscall_cache_t syscall = {
        .type = EVENT_OPEN,
        .policy = policy,
        .async = async,
        .open = {
            .flags = flags,
            .mode = mode & S_IALLUGO,
        }
    };

    if (pid_tgid > 0) {
        syscall.open.pid_tgid = pid_tgid;
    }

    collect_syscall_ctx(&syscall, SYSCALL_CTX_ARG_STR(0) | SYSCALL_CTX_ARG_INT(1) | SYSCALL_CTX_ARG_INT(2), (void *)path, (void *)&flags, (void *)&mode);
    cache_syscall(&syscall);

    return 0;
}

int __attribute__((always_inline)) trace__sys_openat(const char *path, u8 async, int flags, umode_t mode) {
    return trace__sys_openat2(path, async, flags, mode, 0);
}

HOOK_SYSCALL_ENTRY2(creat, const char *, filename, umode_t, mode) {
    int flags = O_CREAT | O_WRONLY | O_TRUNC;
    return trace__sys_openat(filename, SYNC_SYSCALL, flags, mode);
}

HOOK_SYSCALL_COMPAT_ENTRY3(open_by_handle_at, int, mount_fd, struct file_handle *, handle, int, flags) {
    umode_t mode = 0;
    return trace__sys_openat(NULL, SYNC_SYSCALL, flags, mode);
}

HOOK_SYSCALL_COMPAT_ENTRY1(truncate, const char *, filename) {
    int flags = O_CREAT | O_WRONLY | O_TRUNC;
    umode_t mode = 0;
    return trace__sys_openat(filename, SYNC_SYSCALL, flags, mode);
}

HOOK_SYSCALL_COMPAT_ENTRY0(ftruncate) {
    int flags = O_CREAT | O_WRONLY | O_TRUNC;
    umode_t mode = 0;
    char filename[1] = "";
    return trace__sys_openat(filename, SYNC_SYSCALL, flags, mode);
}

HOOK_SYSCALL_COMPAT_ENTRY3(open, const char *, filename, int, flags, umode_t, mode) {
    return trace__sys_openat(filename, SYNC_SYSCALL, flags, mode);
}

HOOK_SYSCALL_COMPAT_ENTRY4(openat, int, dirfd, const char *, filename, int, flags, umode_t, mode) {
    return trace__sys_openat(filename, SYNC_SYSCALL, flags, mode);
}

HOOK_SYSCALL_ENTRY4(openat2, int, dirfd, const char *, filename, struct openat2_open_how *, phow, size_t, size) {
    struct openat2_open_how how;
    bpf_probe_read(&how, sizeof(struct openat2_open_how), phow);
    return trace__sys_openat(filename, SYNC_SYSCALL, how.flags, how.mode);
}

int __attribute__((always_inline)) handle_open(ctx_t *ctx, struct path *path) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_OPEN);
    if (!syscall || syscall->open.dentry) {
        return 0;
    }

    struct dentry *dentry = get_path_dentry(path);
    if (!dentry || is_non_mountable_dentry(dentry)) {
        return 0;
    }

    struct path_key_t path_key = get_dentry_key_path(dentry, path);
    if (path_key.ino == 0) {
        return 0;
    }

    syscall->open.dentry = dentry;
    syscall->open.file.path_key = path_key;

    set_file_inode(dentry, &syscall->open.file, PATH_ID_INVALIDATE_TYPE_NONE);

    // do not pop, we want to keep track of the mount ref counter later in the stack
    enum SYSCALL_STATE state = approve_syscall(syscall, open_approvers);
    if (state == SAMPLED) {
        // fake an activity dump for now, this will avoid discarders
        // we should convert this to a SAMPLE flag
        syscall->resolver.flags |= ACTIVITY_DUMP_RUNNING;
    }

    syscall->resolver.key = syscall->open.file.path_key;
    syscall->resolver.dentry = syscall->open.dentry;
    syscall->resolver.discarder_event_type = dentry_resolver_discarder_event_type(syscall);
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    // tail call
    resolve_dentry(ctx, KPROBE_OR_FENTRY_TYPE);

    return 0;
}

int __attribute__((always_inline)) handle_truncate_path(ctx_t *ctx, struct path *path) {
    if (path == NULL) {
        return 0;
    }
    return handle_open(ctx, path);
}

HOOK_ENTRY("do_truncate")
int hook_do_truncate(ctx_t *ctx) {
    struct file *f = (struct file *)CTX_PARM4(ctx);
    if (f == NULL) {
        return 0;
    }
    return handle_open(ctx, get_file_f_path_addr(f));
}

HOOK_ENTRY("vfs_truncate")
int hook_vfs_truncate(ctx_t *ctx) {
    struct path *path = (struct path *)CTX_PARM1(ctx);
    return handle_open(ctx, path);
}

HOOK_ENTRY("security_file_truncate")
int hook_security_file_truncate(ctx_t *ctx) {
    struct file *f = (struct file *)CTX_PARM1(ctx);
    if (f == NULL) {
        return 0;
    }
    return handle_open(ctx, get_file_f_path_addr(f));
}

HOOK_ENTRY("security_path_truncate")
int hook_security_path_truncate(ctx_t *ctx) {
    struct path *path = (struct path *)CTX_PARM1(ctx);
    return handle_open(ctx, path);
}

HOOK_ENTRY("vfs_open")
int hook_vfs_open(ctx_t *ctx) {
    struct path *path = (struct path *)CTX_PARM1(ctx);
    return handle_open(ctx, path);
}

HOOK_ENTRY("terminate_walk")
int hook_terminate_walk(ctx_t *ctx) {
    struct path *path = (struct path *)CTX_PARM1(ctx);
    return handle_open(ctx, path);
}

HOOK_ENTRY("do_dentry_open")
int hook_do_dentry_open(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_EXEC);
    if (!syscall) {
        return 0;
    }

    struct file *file = (struct file *)CTX_PARM1(ctx);

    u64 do_dentry_open_without_inode;
    LOAD_CONSTANT("do_dentry_open_without_inode", do_dentry_open_without_inode);

    struct inode *inode = NULL;
    if (!do_dentry_open_without_inode) {
        inode = (struct inode *)CTX_PARM2(ctx);
    }

    return handle_exec_event(ctx, syscall, file, inode);
}

int __attribute__((always_inline)) trace_io_openat(ctx_t *ctx) {
    void *raw_req = (void *)CTX_PARM1(ctx);

    struct io_open req;
    if (bpf_probe_read(&req, sizeof(req), raw_req)) {
        return 0;
    }

    u64 pid_tgid = get_pid_tgid_from_iouring(raw_req);

    struct syscall_cache_t *syscall = peek_syscall(EVENT_OPEN);
    if (!syscall) {
        unsigned int flags = req.how.flags & VALID_OPEN_FLAGS;
        umode_t mode = req.how.mode & S_IALLUGO;
        return trace__sys_openat2(NULL, ASYNC_SYSCALL, flags, mode, pid_tgid);
    } else {
        syscall->open.pid_tgid = get_pid_tgid_from_iouring(raw_req);
    }
    return 0;
}

HOOK_ENTRY("io_openat")
int hook_io_openat(ctx_t *ctx) {
    return trace_io_openat(ctx);
}

HOOK_ENTRY("io_openat2")
int hook_io_openat2(ctx_t *ctx) {
    return trace_io_openat(ctx);
}

// used by both tail call callback and directly for tracepoints
int __attribute__((always_inline)) _sys_open_ret(void *ctx, struct syscall_cache_t *syscall) {
    if (IS_UNHANDLED_ERROR(syscall->retval)) {
        return 0;
    }

    // check if the syscall was discarded
    if (syscall->state == DISCARDED) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_OPEN);
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_INVALID) {
        return 0;
    }

    struct open_event_t event = {
        .syscall.retval = syscall->retval,
        .syscall_ctx.id = syscall->ctx_id,
        .event.flags = (syscall->async ? EVENT_FLAGS_ASYNC : 0) |
                       (syscall->resolver.flags & SAVED_BY_ACTIVITY_DUMP ? EVENT_FLAGS_SAVED_BY_AD : 0) |
                       (syscall->resolver.flags & ACTIVITY_DUMP_RUNNING ? EVENT_FLAGS_ACTIVITY_DUMP_SAMPLE : 0),
        .file = syscall->open.file,
        .flags = syscall->open.flags,
        .mode = syscall->open.mode,
    };

    fill_file(syscall->open.dentry, &event.file);

    struct proc_cache_t *entry;
    if (syscall->open.pid_tgid != 0) {
        entry = fill_process_context_with_pid_tgid(&event.process, syscall->open.pid_tgid);
    } else {
        entry = fill_process_context(&event.process);
    }
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_OPEN, event);

    return 0;
}

TAIL_CALL_FNC(sys_open_ret_cb, void *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_OPEN);
    if (!syscall || !syscall->open.dentry) {
        return 0;
    }
    return _sys_open_ret(ctx, syscall);
}

// get and set the retval then tail call so that only one program is used for all the syscall ret
int __attribute__((always_inline)) sys_open_ret(void *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_OPEN);
    if (!syscall) {
        return 0;
    }
    syscall->retval = SYSCALL_PARMRET(ctx);

    bpf_tail_call_compat(ctx, &open_ret_progs, 0);

    return 0;
}

HOOK_SYSCALL_EXIT(creat) {
    return sys_open_ret(ctx);
}

HOOK_SYSCALL_COMPAT_EXIT(open_by_handle_at) {
    return sys_open_ret(ctx);
}

HOOK_SYSCALL_COMPAT_EXIT(truncate) {
    return sys_open_ret(ctx);
}

HOOK_SYSCALL_COMPAT_EXIT(ftruncate) {
    return sys_open_ret(ctx);
}

HOOK_SYSCALL_COMPAT_EXIT(open) {
    return sys_open_ret(ctx);
}

HOOK_SYSCALL_COMPAT_EXIT(openat) {
    return sys_open_ret(ctx);
}

HOOK_SYSCALL_EXIT(openat2) {
    return sys_open_ret(ctx);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_open_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_OPEN);
    if (!syscall || !syscall->open.dentry) {
        return 0;
    }
    syscall->retval = args->ret;
    return _sys_open_ret(args, syscall);
}

HOOK_EXIT("io_openat2")
int rethook_io_openat2(ctx_t *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_OPEN);
    if (!syscall || !syscall->open.dentry) {
        return 0;
    }
    syscall->retval = CTX_PARMRET(ctx);
    return _sys_open_ret(ctx, syscall);
}

#endif
