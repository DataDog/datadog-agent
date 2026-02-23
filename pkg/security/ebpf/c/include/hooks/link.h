#ifndef _HOOKS_LINK_H_
#define _HOOKS_LINK_H_

#include "constants/syscall_macro.h"
#include "helpers/approvers.h"
#include "helpers/discarders.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) trace__sys_link(u8 async, const char *oldpath, const char *newpath) {
    struct policy_t policy = fetch_policy(EVENT_LINK);
    struct syscall_cache_t syscall = {
        .type = EVENT_LINK,
        .policy = policy,
        .async = async,
    };

    if (!async) {
        collect_syscall_ctx(&syscall, SYSCALL_CTX_ARG_STR(0) | SYSCALL_CTX_ARG_STR(1), (void *)oldpath, (void *)newpath, NULL);
    }
    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_ENTRY2(link, const char *, oldpath, const char *, newpath) {
    return trace__sys_link(SYNC_SYSCALL, oldpath, newpath);
}

HOOK_SYSCALL_ENTRY4(linkat, int, olddirfd, const char *, oldpath, int, newdirfd, const char *, newpath) {
    return trace__sys_link(SYNC_SYSCALL, oldpath, newpath);
}

HOOK_ENTRY("do_linkat")
int hook_do_linkat(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_LINK);
    if (!syscall) {
        return trace__sys_link(ASYNC_SYSCALL, NULL, NULL);
    }
    return 0;
}

HOOK_ENTRY("complete_walk")
int hook_complete_walk(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_LINK);
    if (!syscall) {
        return 0;
    }

    if (syscall->link.src_path) {
        return 0;
    }

    // struct path is the first field of struct nameidata
    syscall->link.src_path = (struct path *)CTX_PARM1(ctx);
    struct dentry *src_dentry = get_path_dentry(syscall->link.src_path);
    syscall->link.src_dentry = src_dentry;

    syscall->link.src_file.path_key.mount_id = get_path_mount_id(syscall->link.src_path);

    // force a new path id to force path resolution
    set_file_inode(src_dentry, &syscall->link.src_file, PATH_ID_INVALIDATE_TYPE_LOCAL);
    fill_file(src_dentry, &syscall->link.src_file);

    syscall->resolver.dentry = src_dentry;
    syscall->resolver.key = syscall->link.src_file.path_key;
    syscall->resolver.discarder_event_type = dentry_resolver_discarder_event_type(syscall);
    syscall->resolver.callback = DR_LINK_SRC_CALLBACK_KPROBE_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, KPROBE_OR_FENTRY_TYPE);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_LINK);

    return 0;
}

TAIL_CALL_FNC(dr_link_src_callback, ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_LINK);
    if (!syscall) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_LINK);
        // do not pop, we want to invalidate the inode even if the syscall is discarded
        syscall->state = DISCARDED;
    }

    return 0;
}

int __attribute__((always_inline)) create_link_target_dentry_common(struct dentry *target_dentry, enum link_target_dentry_origin origin) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_LINK);
    if (!syscall) {
        return 0;
    }

    // The goal of this state machine is to handle the combination of cases where this function is called multiple times:
    // - when __lookup_hash is called multiple times (e.g. when overlayfs is used)
    // - when filename_create is called multiple times (e.g. when overlayfs is used)
    // - when both filename_create and __lookup_hash return hook points are loaded
    // - when both filename_create and __lookup_hash return hook points are loaded and overlayfs is used
    // in all of these cases:
    // - we only care about the last call to __lookup_hash
    // - or we only care about the first call to filename_create
    switch (syscall->link.target_dentry_origin) {
    case ORIGIN_UNSET:
        // set target dentry unconditionally if it was not set before
        // fallthrough
    case ORIGIN_RETHOOK___LOOKUP_HASH:
        // overwrite the target dentry only if it was set by __lookup_hash
        syscall->link.target_dentry = target_dentry;
        syscall->link.target_dentry_origin = origin;
        break;
    case ORIGIN_RETHOOK_FILENAME_CREATE:
        // do not overwrite the target dentry if it was set by filename_create
        break;
    }

    return 0;
}

HOOK_EXIT("filename_create")
int rethook_filename_create(ctx_t *ctx) {
    return create_link_target_dentry_common((struct dentry *)CTX_PARMRET(ctx), ORIGIN_RETHOOK_FILENAME_CREATE);
}

HOOK_EXIT("__lookup_hash")
int rethook___lookup_hash(ctx_t *ctx) {
    return create_link_target_dentry_common((struct dentry *)CTX_PARMRET(ctx), ORIGIN_RETHOOK___LOOKUP_HASH);
}

int __attribute__((always_inline)) sys_link_ret(void *ctx, int retval, enum TAIL_CALL_PROG_TYPE prog_type) {
    if (IS_UNHANDLED_ERROR(retval)) {
        pop_syscall(EVENT_LINK);
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_LINK);
    if (!syscall) {
        return 0;
    }

    // invalidate user space inode, so no need to bump the discarder revision in the event
    if (retval >= 0) {
        // for hardlink we need to invalidate the discarders as the nlink counter in now > 1
        expire_inode_discarders(syscall->link.src_file.path_key.mount_id, syscall->link.src_file.path_key.ino);
    }

    // at this point we have both the source and target dentry so we can check for approvers
    syscall->state = approve_syscall(syscall, link_approvers);

    if (syscall->state != DISCARDED && is_event_enabled(EVENT_LINK)) {
        syscall->retval = retval;

        syscall->link.target_file.metadata = syscall->link.src_file.metadata;

        // we generate a fake target key as the inode is the same
        syscall->link.target_file.path_key.ino = FAKE_INODE_MSW << 32 | bpf_get_prandom_u32();
        // this is a hard link, source and target dentries are on the same filesystem & mount point
        syscall->link.target_file.path_key.mount_id = syscall->link.src_file.path_key.mount_id;
        if (is_overlayfs(syscall->link.src_dentry)) {
            syscall->link.target_file.flags |= UPPER_LAYER;
        }

        syscall->resolver.dentry = syscall->link.target_dentry;
        syscall->resolver.key = syscall->link.target_file.path_key;
        syscall->resolver.discarder_event_type = 0;
        syscall->resolver.callback = select_dr_key(prog_type, DR_LINK_DST_CALLBACK_KPROBE_KEY, DR_LINK_DST_CALLBACK_TRACEPOINT_KEY);
        syscall->resolver.iteration = 0;
        syscall->resolver.ret = 0;

        resolve_dentry(ctx, prog_type);
    }

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_LINK);
    return 0;
}

HOOK_EXIT("do_linkat")
int rethook_do_linkat(ctx_t *ctx) {
    int retval = CTX_PARMRET(ctx);
    return sys_link_ret(ctx, retval, KPROBE_OR_FENTRY_TYPE);
}

HOOK_SYSCALL_EXIT(link) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_link_ret(ctx, retval, KPROBE_OR_FENTRY_TYPE);
}

HOOK_SYSCALL_EXIT(linkat) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_link_ret(ctx, retval, KPROBE_OR_FENTRY_TYPE);
}

TAIL_CALL_TRACEPOINT_FNC(handle_sys_link_exit, struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_link_ret(args, args->ret, TRACEPOINT_TYPE);
}

int __attribute__((always_inline)) dr_link_dst_callback(void *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_LINK);
    if (!syscall) {
        return 0;
    }

    s64 retval = syscall->retval;

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct link_event_t event = {
        .event.type = EVENT_LINK,
        .event.timestamp = bpf_ktime_get_ns(),
        .syscall.retval = retval,
        .syscall_ctx.id = syscall->ctx_id,
        .event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0,
        .source = syscall->link.src_file,
        .target = syscall->link.target_file,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_cgroup_context(entry, &event.cgroup);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_LINK, event);

    return 0;
}

TAIL_CALL_FNC(dr_link_dst_callback, ctx_t *ctx) {
    return dr_link_dst_callback(ctx);
}

TAIL_CALL_TRACEPOINT_FNC(dr_link_dst_callback, struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_link_dst_callback(args);
}

#endif
