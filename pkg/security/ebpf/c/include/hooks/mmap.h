#ifndef _HOOKS_MMAP_H_
#define _HOOKS_MMAP_H_

#include "constants/syscall_macro.h"
#include "helpers/approvers.h"
#include "helpers/discarders.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

SEC("tracepoint/syscalls/sys_enter_mmap")
int tracepoint_syscalls_sys_enter_mmap(struct tracepoint_syscalls_sys_enter_mmap_t *args) {
    struct policy_t policy = fetch_policy(EVENT_MMAP);
    if (is_discarded_by_process(policy.mode, EVENT_MMAP)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_MMAP,
        .mmap = {
            .offset = args->offset,
            .len = (u32)args->len,
            .protection = (int)args->protection,
            .flags = (int)args->flags,
        }
    };

    cache_syscall(&syscall);
    return 0;
}

int __attribute__((always_inline)) sys_mmap_ret(void *ctx, int retval, u64 addr) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_MMAP);
    if (!syscall) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_MMAP);
        return 0;
    }

    if (filter_syscall(syscall, mmap_approvers)) {
        return discard_syscall(syscall);
    }

    if (retval != -1) {
        retval = 0;
    }

    struct mmap_event_t event = {
        .syscall.retval = retval,
        .file = syscall->mmap.file,
        .addr = addr,
        .offset = syscall->mmap.offset,
        .len = syscall->mmap.len,
        .protection = syscall->mmap.protection,
        .flags = syscall->mmap.flags,
    };

    if (syscall->mmap.dentry != NULL) {
        fill_file(syscall->mmap.dentry, &event.file);
    }
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_MMAP, event);
    return 0;
}

HOOK_SYSCALL_EXIT(mmap) {
    return sys_mmap_ret(ctx, (int)SYSCALL_PARMRET(ctx), (u64)SYSCALL_PARMRET(ctx));
}

HOOK_EXIT("fget")
int rethook_fget(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MMAP);
    if (!syscall) {
        return 0;
    }

    struct file *f = (struct file*) CTX_PARMRET(ctx, 1);
    syscall->mmap.dentry = get_file_dentry(f);
    set_file_inode(syscall->mmap.dentry, &syscall->mmap.file, 0);
    syscall->mmap.file.path_key.mount_id = get_file_mount_id(f);

    syscall->resolver.key = syscall->mmap.file.path_key;
    syscall->resolver.dentry = syscall->mmap.dentry;
    syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? EVENT_MMAP : 0;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE_OR_FENTRY);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_MMAP);

    return 0;
}

SEC("tracepoint/handle_sys_mmap_exit")
int tracepoint_handle_sys_mmap_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_mmap_ret(args, (int)args->ret, (u64)args->ret);
}

#endif
