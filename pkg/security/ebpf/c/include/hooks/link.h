#ifndef _HOOKS_LINK_H_
#define _HOOKS_LINK_H_

#include "constants/syscall_macro.h"
#include "helpers/approvers.h"
#include "helpers/discarders.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) trace__sys_link(u8 async) {
    struct policy_t policy = fetch_policy(EVENT_LINK);
    if (is_discarded_by_process(policy.mode, EVENT_LINK)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_LINK,
        .policy = policy,
        .async = async,
    };

    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_ENTRY0(link) {
    return trace__sys_link(SYNC_SYSCALL);
}

HOOK_SYSCALL_ENTRY0(linkat) {
    return trace__sys_link(SYNC_SYSCALL);
}

HOOK_ENTRY("do_linkat")
int hook_do_linkat(ctx_t *ctx) {
    struct syscall_cache_t* syscall = peek_syscall(EVENT_LINK);
    if (!syscall) {
        return trace__sys_link(ASYNC_SYSCALL);
    }
    return 0;
}

HOOK_ENTRY("vfs_link")
int hook_vfs_link(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_LINK);
    if (!syscall) {
        return 0;
    }

    if (syscall->link.target_dentry) {
        return 0;
    }

    struct dentry *src_dentry = (struct dentry *)CTX_PARM1(ctx);
    syscall->link.src_dentry = src_dentry;

    syscall->link.target_dentry = (struct dentry *)CTX_PARM3(ctx);
    // change the register based on the value of vfs_link_target_dentry_position
    if (get_vfs_link_target_dentry_position() == VFS_ARG_POSITION4) {
        // prevent the verifier from whining
        bpf_probe_read(&syscall->link.target_dentry, sizeof(syscall->link.target_dentry), &syscall->link.target_dentry);
        syscall->link.target_dentry = (struct dentry *) CTX_PARM4(ctx);
    }

    // this is a hard link, source and target dentries are on the same filesystem & mount point
    // target_path was set by kprobe/filename_create before we reach this point.
    syscall->link.src_file.path_key.mount_id = get_path_mount_id(syscall->link.target_path);

    // force a new path id to force path resolution
    set_file_inode(src_dentry, &syscall->link.src_file, 1);

    if (filter_syscall(syscall, link_approvers)) {
        return mark_as_discarded(syscall);
    }

    fill_file(src_dentry, &syscall->link.src_file);
    syscall->link.target_file.metadata = syscall->link.src_file.metadata;

    // we generate a fake target key as the inode is the same
    syscall->link.target_file.path_key.ino = FAKE_INODE_MSW<<32 | bpf_get_prandom_u32();
    syscall->link.target_file.path_key.mount_id = syscall->link.src_file.path_key.mount_id;
    if (is_overlayfs(src_dentry)) {
        syscall->link.target_file.flags |= UPPER_LAYER;
    }

    syscall->resolver.dentry = src_dentry;
    syscall->resolver.key = syscall->link.src_file.path_key;
    syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? EVENT_LINK : 0;
    syscall->resolver.callback = DR_LINK_SRC_CALLBACK_KPROBE_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE_OR_FENTRY);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_LINK);

    return 0;
}

TAIL_CALL_TARGET("dr_link_src_callback")
int tail_call_target_dr_link_src_callback(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_LINK);
    if (!syscall) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_LINK);
        return mark_as_discarded(syscall);
    }

    return 0;
}

int __attribute__((always_inline)) sys_link_ret(void *ctx, int retval, int dr_type) {
    if (IS_UNHANDLED_ERROR(retval)) {
        pop_syscall(EVENT_LINK);
        return 0;
    }

    struct syscall_cache_t *syscall = peek_syscall(EVENT_LINK);
    if (!syscall) {
        return 0;
    }

    int pass_to_userspace = !syscall->discarded && is_event_enabled(EVENT_LINK);

    // invalidate user space inode, so no need to bump the discarder revision in the event
    if (retval >= 0) {
        // for hardlink we need to invalidate the discarders as the nlink counter in now > 1
        expire_inode_discarders(syscall->link.src_file.path_key.mount_id, syscall->link.src_file.path_key.ino);
    }

    if (pass_to_userspace) {
        syscall->resolver.dentry = syscall->link.target_dentry;
        syscall->resolver.key = syscall->link.target_file.path_key;
        syscall->resolver.discarder_type = 0;
        syscall->resolver.callback = select_dr_key(dr_type, DR_LINK_DST_CALLBACK_KPROBE_KEY, DR_LINK_DST_CALLBACK_TRACEPOINT_KEY);
        syscall->resolver.iteration = 0;
        syscall->resolver.ret = 0;
        syscall->resolver.sysretval = retval;

        resolve_dentry(ctx, dr_type);
    }

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_LINK);
    return 0;
}

HOOK_EXIT("do_linkat")
int rethook_do_linkat(ctx_t *ctx) {
    int retval = CTX_PARMRET(ctx, 5);
    return sys_link_ret(ctx, retval, DR_KPROBE_OR_FENTRY);
}

HOOK_SYSCALL_EXIT(link) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_link_ret(ctx, retval, DR_KPROBE_OR_FENTRY);
}

HOOK_SYSCALL_EXIT(linkat) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_link_ret(ctx, retval, DR_KPROBE_OR_FENTRY);
}

SEC("tracepoint/handle_sys_link_exit")
int tracepoint_handle_sys_link_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_link_ret(args, args->ret, DR_TRACEPOINT);
}

int __attribute__((always_inline)) dr_link_dst_callback(void *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_LINK);
    if (!syscall) {
        return 0;
    }

    s64 retval = syscall->resolver.sysretval;

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct link_event_t event = {
        .event.type = EVENT_LINK,
        .event.timestamp = bpf_ktime_get_ns(),
        .syscall.retval = retval,
        .event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0,
        .source = syscall->link.src_file,
        .target = syscall->link.target_file,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_LINK, event);

    return 0;
}

TAIL_CALL_TARGET("dr_link_dst_callback")
int tail_call_target_dr_link_dst_callback(ctx_t *ctx) {
    return dr_link_dst_callback(ctx);
}

SEC("tracepoint/dr_link_dst_callback")
int tracepoint_dr_link_dst_callback(struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_link_dst_callback(args);
}

#endif
