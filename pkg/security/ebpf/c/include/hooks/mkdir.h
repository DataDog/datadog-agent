#ifndef _HOOKS_MKDIR_H_
#define _HOOKS_MKDIR_H_

#include "constants/syscall_macro.h"
#include "helpers/approvers.h"
#include "helpers/discarders.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

long __attribute__((always_inline)) trace__sys_mkdir(u8 async, const char *filename, umode_t mode) {
    if (is_discarded_by_pid()) {
        return 0;
    }

    struct policy_t policy = fetch_policy(EVENT_MKDIR);
    struct syscall_cache_t syscall = {
        .type = EVENT_MKDIR,
        .policy = policy,
        .async = async,
        .mkdir = {
            .mode = mode }
    };

    if (!async) {
        collect_syscall_ctx(&syscall, SYSCALL_CTX_ARG_STR(0) | SYSCALL_CTX_ARG_INT(1), (void *)filename, (void *)&mode, NULL);
    }
    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_ENTRY2(mkdir, const char *, filename, umode_t, mode) {
    return trace__sys_mkdir(SYNC_SYSCALL, filename, mode);
}

HOOK_SYSCALL_ENTRY3(mkdirat, int, dirfd, const char *, filename, umode_t, mode) {
    return trace__sys_mkdir(SYNC_SYSCALL, filename, mode);
}

int __attribute__((always_inline)) filename_create_common(struct path *p) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MKDIR);
    if (!syscall) {
        return 0;
    }

    syscall->mkdir.path = p;

    return 0;
}

HOOK_ENTRY("filename_create")
int hook_filename_create(ctx_t *ctx) {
    struct path *p = (struct path *)CTX_PARM3(ctx);
    return filename_create_common(p);
}

HOOK_ENTRY("security_path_mkdir")
int hook_security_path_mkdir(ctx_t *ctx) {
    struct path *p = (struct path *)CTX_PARM1(ctx);
    return filename_create_common(p);
}

HOOK_ENTRY("vfs_mkdir")
int hook_vfs_mkdir(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MKDIR);
    if (!syscall) {
        return 0;
    }

    if (syscall->mkdir.dentry) {
        return 0;
    }

    syscall->mkdir.dentry = (struct dentry *)CTX_PARM2(ctx);
    // change the register based on the value of vfs_mkdir_dentry_position
    if (get_vfs_mkdir_dentry_position() == VFS_ARG_POSITION3) {
        // prevent the verifier from whining
        bpf_probe_read(&syscall->mkdir.dentry, sizeof(syscall->mkdir.dentry), &syscall->mkdir.dentry);
        syscall->mkdir.dentry = (struct dentry *)CTX_PARM3(ctx);
    }

    syscall->mkdir.file.path_key.mount_id = get_path_mount_id(syscall->mkdir.path);

    if (approve_syscall(syscall, mkdir_approvers) == DISCARDED) {
        pop_syscall(EVENT_MKDIR);
    }

    return 0;
}

int __attribute__((always_inline)) sys_mkdir_ret(void *ctx, int retval, enum TAIL_CALL_PROG_TYPE prog_type) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MKDIR);
    if (!syscall) {
        return 0;
    }
    if (IS_UNHANDLED_ERROR(retval)) {
        pop_syscall(EVENT_MKDIR);
        return 0;
    }

    // the inode of the dentry was not properly set when kprobe/security_path_mkdir was called, make sure we grab it now
    set_file_inode(syscall->mkdir.dentry, &syscall->mkdir.file, PATH_ID_INVALIDATE_TYPE_NONE);

    if (retval && !syscall->mkdir.file.path_key.ino) {
        syscall->mkdir.file.path_key.mount_id = 0; // do not try resolving the path
    }

    syscall->retval = retval;

    syscall->resolver.key = syscall->mkdir.file.path_key;
    syscall->resolver.dentry = syscall->mkdir.dentry;
    syscall->resolver.discarder_event_type = dentry_resolver_discarder_event_type(syscall);
    syscall->resolver.callback = select_dr_key(prog_type, DR_MKDIR_CALLBACK_KPROBE_KEY, DR_MKDIR_CALLBACK_TRACEPOINT_KEY);
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, prog_type);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_MKDIR);
    return 0;
}

HOOK_ENTRY("do_mkdirat")
int hook_do_mkdirat(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MKDIR);
    if (!syscall) {
        umode_t mode = (umode_t)CTX_PARM3(ctx);
        return trace__sys_mkdir(ASYNC_SYSCALL, NULL, mode);
    }
    return 0;
}

HOOK_EXIT("do_mkdirat")
int rethook_do_mkdirat(ctx_t *ctx) {
    int retval = CTX_PARMRET(ctx);
    return sys_mkdir_ret(ctx, retval, KPROBE_OR_FENTRY_TYPE);
}

HOOK_SYSCALL_EXIT(mkdir) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_mkdir_ret(ctx, retval, KPROBE_OR_FENTRY_TYPE);
}

HOOK_SYSCALL_EXIT(mkdirat) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_mkdir_ret(ctx, retval, KPROBE_OR_FENTRY_TYPE);
}


TAIL_CALL_TRACEPOINT_FNC(handle_sys_mkdir_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_mkdir_ret(args, args->ret, TRACEPOINT_TYPE);
}

int __attribute__((always_inline)) dr_mkdir_callback(void *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_MKDIR);
    if (!syscall) {
        return 0;
    }

    s64 retval = syscall->retval;

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_MKDIR);
        return 0;
    }

    struct mkdir_event_t event = {
        .syscall.retval = retval,
        .syscall_ctx.id = syscall->ctx_id,
        .event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0,
        .file = syscall->mkdir.file,
        .mode = syscall->mkdir.mode,
    };

    fill_file(syscall->mkdir.dentry, &event.file);
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_MKDIR, event);
    return 0;
}

TAIL_CALL_FNC(dr_mkdir_callback, ctx_t *ctx) {
    return dr_mkdir_callback(ctx);
}

TAIL_CALL_TRACEPOINT_FNC(dr_mkdir_callback, struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_mkdir_callback(args);
}

#endif
