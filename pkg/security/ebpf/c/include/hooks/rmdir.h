#ifndef _HOOKS_RMDIR_H_
#define _HOOKS_RMDIR_H_

#include "constants/syscall_macro.h"
#include "helpers/approvers.h"
#include "helpers/events_predicates.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"
#include "helpers/discarders.h"

int __attribute__((always_inline)) trace__sys_rmdir(void *ctx, u8 async, const char *filename) {
    struct syscall_cache_t syscall = {
        .type = EVENT_RMDIR,
        .policy = fetch_policy(EVENT_RMDIR),
        .async = async,
    };

    if (!async) {
        collect_syscall_ctx(&syscall, SYSCALL_CTX_ARG_STR(0), (void *)filename, NULL, NULL);
    }
    cache_syscall_update_cgroup(ctx, &syscall);
    return 0;
}

HOOK_SYSCALL_ENTRY1(rmdir, const char *, filename) {
    return trace__sys_rmdir(ctx, SYNC_SYSCALL, filename);
}

HOOK_ENTRY("do_rmdir")
int hook_do_rmdir(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(rmdir_predicate);
    if (!syscall) {
        return trace__sys_rmdir(ctx, ASYNC_SYSCALL, NULL);
    }
    return 0;
}

HOOK_ENTRY("filename_rmdir")
int hook_filename_rmdir(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(rmdir_predicate);
    if (!syscall) {
        return trace__sys_rmdir(ctx, ASYNC_SYSCALL, NULL);
    }
    return 0;
}

// security_inode_rmdir is shared between rmdir and unlink syscalls
HOOK_ENTRY("security_inode_rmdir")
int hook_security_inode_rmdir(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(rmdir_predicate);
    if (!syscall) {
        return 0;
    }

    struct path_key_t key = {};
    struct dentry *dentry = NULL;
    u8 is_cgroup_dentry = 0;

    switch (syscall->type) {
    case EVENT_RMDIR:
        if (syscall->rmdir.file.path_key.ino) {
            return 0;
        }

        // we resolve all the information before the file is actually removed
        dentry = (struct dentry *)CTX_PARM2(ctx);
        set_file_inode(dentry, &syscall->rmdir.file, PATH_ID_INVALIDATE_TYPE_GLOBAL);
        fill_file(dentry, &syscall->rmdir.file);

        // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
        key = syscall->rmdir.file.path_key;

        syscall->rmdir.dentry = dentry;

        is_cgroup_dentry = is_cgroup2fs(syscall->rmdir.dentry) && S_ISDIR(syscall->rmdir.file.metadata.mode) && !is_runtime_request();

        break;
    case EVENT_UNLINK:
        if (syscall->unlink.file.path_key.ino) {
            return 0;
        }

        // we resolve all the information before the file is actually removed
        dentry = (struct dentry *)CTX_PARM2(ctx);
        set_file_inode(dentry, &syscall->unlink.file, PATH_ID_INVALIDATE_TYPE_GLOBAL);
        fill_file(dentry, &syscall->unlink.file);

        // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
        key = syscall->unlink.file.path_key;

        syscall->unlink.dentry = dentry;

        is_cgroup_dentry = is_cgroup2fs(syscall->unlink.dentry) && S_ISDIR(syscall->unlink.file.metadata.mode) && !is_runtime_request();

        break;
    default:
        return 0;
    }

    // force the policy to RMDIR — both rmdir and unlink-with-AT_REMOVEDIR are
    // evaluated against the rmdir approvers below.
    syscall->policy = fetch_policy(EVENT_RMDIR);

    approve_syscall(syscall, rmdir_approvers);

    // let the cgroup event being forwarded as it is used userspace side to track the cgroups
    if (syscall->state != ACCEPTED && is_cgroup_dentry) {
        syscall->state = INTERNAL;
    }

    if (dentry != NULL) {
        syscall->resolver.key = key;
        syscall->resolver.dentry = dentry;
        // force the resolver event_type to EVENT_RMDIR: unlink with AT_REMOVEDIR is processed as an rmdir userspace side
        syscall->resolver.event_type = EVENT_RMDIR;
        // disable the dentry-resolver discarder for cgroupfs events: userspace needs them
        // to track cgroup lifecycle, and a discarder match here would drop them.
        syscall->resolver.flags = get_resolver_flags(syscall, !is_cgroup_dentry);
        syscall->resolver.callback = DR_SECURITY_INODE_RMDIR_CALLBACK_KPROBE_KEY;
        syscall->resolver.iteration = 0;
        syscall->resolver.ret = 0;

        resolve_dentry(ctx, KPROBE_OR_FENTRY_TYPE);

        // if the tail call fails, we need to pop the syscall cache entry
        pop_syscall_with(rmdir_predicate);
    }
    return 0;
}

TAIL_CALL_FNC(dr_security_inode_rmdir_callback, ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(rmdir_predicate);
    if (!syscall) {
        return 0;
    }

    // force the resolver event_type to EVENT_RMDIR: unlink with AT_REMOVEDIR is processed as an rmdir userspace side
    apply_dentry_resolution_outcome(syscall, EVENT_RMDIR);

    return 0;
}

int __attribute__((always_inline)) sys_rmdir_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall_with(rmdir_predicate);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    if (syscall->state != DISCARDED && is_auid_discarder(EVENT_RMDIR)) {
        syscall->state = DISCARDED;
        monitor_discarded(EVENT_RMDIR);
    }

    if (syscall->state != DISCARDED) {
        struct rmdir_event_t event = {
            .syscall.retval = retval,
            .syscall_ctx.id = syscall->ctx_id,
            .event.flags = (syscall->async ? EVENT_FLAGS_ASYNC : 0) |
                           (syscall->state == INTERNAL ? EVENT_FLAGS_INTERNAL : 0),
            .file = syscall->rmdir.file,
        };

        struct proc_cache_t *entry = fill_process_context(&event.process);
        fill_cgroup_context(entry, &event.cgroup);
        fill_span_context(&event.span);

        send_event(ctx, EVENT_RMDIR, event);
    }

    if (retval >= 0) {
        expire_inode_discarders(syscall->rmdir.file.path_key.mount_id, syscall->rmdir.file.path_key.ino);
    }

    return 0;
}

HOOK_EXIT("do_rmdir")
int rethook_do_rmdir(ctx_t *ctx) {
    int retval = CTX_PARMRET(ctx);
    return sys_rmdir_ret(ctx, retval);
}

HOOK_EXIT("filename_rmdir")
int rethook_filename_rmdir(ctx_t *ctx) {
    int retval = CTX_PARMRET(ctx);
    return sys_rmdir_ret(ctx, retval);
}

HOOK_SYSCALL_EXIT(rmdir) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_rmdir_ret(ctx, retval);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_rmdir_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_rmdir_ret(args, args->ret);
}

#endif
