#ifndef _HOOKS_SPLICE_H_
#define _HOOKS_SPLICE_H_

#include "constants/offsets/splice.h"
#include "constants/syscall_macro.h"
#include "helpers/approvers.h"
#include "helpers/discarders.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

HOOK_SYSCALL_ENTRY0(splice) {
    struct policy_t policy = fetch_policy(EVENT_SPLICE);
    if (is_discarded_by_process(policy.mode, EVENT_SPLICE)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_SPLICE,
    };

    cache_syscall(&syscall);
    return 0;
}

HOOK_ENTRY("get_pipe_info")
int hook_get_pipe_info(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SPLICE);
    if (!syscall) {
        return 0;
    }

    // resolve the "in" file path
    if (!syscall->splice.file_found) {
        struct file *f = (struct file*) CTX_PARM1(ctx);
        syscall->splice.dentry = get_file_dentry(f);
        set_file_inode(syscall->splice.dentry, &syscall->splice.file, 0);
        syscall->splice.file.path_key.mount_id = get_file_mount_id(f);
    }

    return 0;
}

HOOK_EXIT("get_pipe_info")
int rethook_get_pipe_info(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_SPLICE);
    if (!syscall) {
        return 0;
    }

    struct pipe_inode_info *info = (struct pipe_inode_info *)CTX_PARMRET(ctx, 2);
    if (info == NULL) {
        // this is not a pipe, so most likely a file, resolve its path now
        syscall->splice.file_found = 1;
        syscall->resolver.key = syscall->splice.file.path_key;
        syscall->resolver.dentry = syscall->splice.dentry;
        syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? EVENT_SPLICE : 0;
        syscall->resolver.iteration = 0;
        syscall->resolver.ret = 0;

        resolve_dentry(ctx, DR_KPROBE_OR_FENTRY);

        // if the tail call fails, we need to pop the syscall cache entry
        pop_syscall(EVENT_SPLICE);

        return 0;
    }

    bpf_probe_read(&syscall->splice.bufs, sizeof(syscall->splice.bufs), (void *)info + get_pipe_inode_info_bufs_offset());
    if (syscall->splice.bufs != NULL) {
        syscall->splice.pipe_info = info;
        // read the entry flag of the pipe
        syscall->splice.pipe_entry_flag = get_pipe_last_buffer_flags(syscall->splice.pipe_info, syscall->splice.bufs);
    }
    return 0;
}

int __attribute__((always_inline)) sys_splice_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_SPLICE);
    if (!syscall) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_SPLICE);
        return 0;
    }

    if (syscall->splice.pipe_info != NULL && syscall->splice.bufs != NULL) {
        // read the pipe exit flag
        syscall->splice.pipe_exit_flag = get_pipe_last_buffer_flags(syscall->splice.pipe_info, syscall->splice.bufs);
    }

    if (filter_syscall(syscall, splice_approvers)) {
        return discard_syscall(syscall);
    }

    struct splice_event_t event = {
        .syscall.retval = retval,
        .file = syscall->splice.file,
        .pipe_entry_flag = syscall->splice.pipe_entry_flag,
        .pipe_exit_flag = syscall->splice.pipe_exit_flag,
    };
    fill_file(syscall->splice.dentry, &event.file);

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_SPLICE, event);

    return 0;
}

HOOK_SYSCALL_EXIT(splice) {
    return sys_splice_ret(ctx, (int)SYSCALL_PARMRET(ctx));
}

SEC("tracepoint/handle_sys_splice_exit")
int tracepoint_handle_sys_splice_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_splice_ret(args, args->ret);
}

#endif
