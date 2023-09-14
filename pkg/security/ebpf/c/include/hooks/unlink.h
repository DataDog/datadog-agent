#ifndef _HOOKS_UNLINK_H_
#define _HOOKS_UNLINK_H_

#include "constants/syscall_macro.h"
#include "constants/offsets/filesystem.h"
#include "helpers/approvers.h"
#include "helpers/discarders.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

int __attribute__((always_inline)) trace__sys_unlink(u8 async, int flags) {
    struct syscall_cache_t syscall = {
        .type = EVENT_UNLINK,
        .policy = fetch_policy(EVENT_UNLINK),
        .async = async,
        .unlink = {
            .flags = flags,
        }
    };

    cache_syscall(&syscall);

    return 0;
}

HOOK_SYSCALL_ENTRY0(unlink) {
    return trace__sys_unlink(SYNC_SYSCALL, 0);
}

HOOK_SYSCALL_ENTRY3(unlinkat, int, dirfd, const char*, filename, int, flags) {
    return trace__sys_unlink(SYNC_SYSCALL, flags);
}

HOOK_ENTRY("do_unlinkat")
int hook_do_unlinkat(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNLINK);
    if (!syscall) {
        return trace__sys_unlink(ASYNC_SYSCALL, 0);
    }
    return 0;
}

HOOK_ENTRY("vfs_unlink")
int hook_vfs_unlink(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNLINK);
    if (!syscall) {
        return 0;
    }

    if (syscall->unlink.file.path_key.ino) {
        return 0;
    }

    struct dentry *dentry = (struct dentry *) CTX_PARM2(ctx);
    // change the register based on the value of vfs_unlink_dentry_position
    if (get_vfs_unlink_dentry_position() == VFS_ARG_POSITION3) {
        // prevent the verifier from whining
        bpf_probe_read(&dentry, sizeof(dentry), &dentry);
        dentry = (struct dentry *) CTX_PARM3(ctx);
    }

    // we resolve all the information before the file is actually removed
    syscall->unlink.dentry = dentry;
    set_file_inode(dentry, &syscall->unlink.file, 1);
    fill_file(dentry, &syscall->unlink.file);

    if (filter_syscall(syscall, unlink_approvers)) {
        return mark_as_discarded(syscall);
    }

    if (is_discarded_by_process(syscall->policy.mode, EVENT_UNLINK)) {
        return mark_as_discarded(syscall);
    }

    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    syscall->resolver.dentry = dentry;
    syscall->resolver.key = syscall->unlink.file.path_key;
    syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? EVENT_UNLINK : 0;
    syscall->resolver.callback = DR_UNLINK_CALLBACK_KPROBE_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE_OR_FENTRY);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_UNLINK);

    return 0;
}

TAIL_CALL_TARGET("dr_unlink_callback")
int tail_call_target_dr_unlink_callback(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNLINK);
    if (!syscall) {
        return 0;
    }

    if (syscall->resolver.ret < 0) {
        return mark_as_discarded(syscall);
    }

    return 0;
}

int __attribute__((always_inline)) sys_unlink_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_UNLINK);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    u64 enabled_events = get_enabled_events();
    int pass_to_userspace = !syscall->discarded &&
                            (mask_has_event(enabled_events, EVENT_UNLINK) ||
                             mask_has_event(enabled_events, EVENT_RMDIR));
    if (pass_to_userspace) {
        if (syscall->unlink.flags & AT_REMOVEDIR) {
            struct rmdir_event_t event = {
                .syscall.retval = retval,
                .event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0,
                .file = syscall->unlink.file,
            };

            struct proc_cache_t *entry = fill_process_context(&event.process);
            fill_container_context(entry, &event.container);
            fill_span_context(&event.span);

            send_event(ctx, EVENT_RMDIR, event);
        } else {
            struct unlink_event_t event = {
                .syscall.retval = retval,
                .event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0,
                .file = syscall->unlink.file,
                .flags = syscall->unlink.flags,
            };

            struct proc_cache_t *entry = fill_process_context(&event.process);
            fill_container_context(entry, &event.container);
            fill_span_context(&event.span);

            send_event(ctx, EVENT_UNLINK, event);
        }
    } else {
        if (mask_has_event(enabled_events, EVENT_RMDIR)) {
            monitor_discarded(EVENT_RMDIR);
        } else {
            monitor_discarded(EVENT_UNLINK);
        }
    }

    if (retval >= 0) {
        expire_inode_discarders(syscall->unlink.file.path_key.mount_id, syscall->unlink.file.path_key.ino);
    }

    return 0;
}

HOOK_EXIT("do_unlinkat")
int rethook_do_unlinkat(ctx_t *ctx) {
    int retval = CTX_PARMRET(ctx, 2);
    return sys_unlink_ret(ctx, retval);
}

HOOK_SYSCALL_EXIT(unlink) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_unlink_ret(ctx, retval);
}

HOOK_SYSCALL_EXIT(unlinkat) {
    int retval = SYSCALL_PARMRET(ctx);
    return sys_unlink_ret(ctx, retval);
}

SEC("tracepoint/handle_sys_unlink_exit")
int tracepoint_handle_sys_unlink_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_unlink_ret(args, args->ret);
}

#endif
