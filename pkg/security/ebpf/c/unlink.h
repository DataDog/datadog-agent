#ifndef _UNLINK_H_
#define _UNLINK_H_

#include "syscalls.h"
#include "process.h"

struct unlink_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    u32 flags;
    u32 padding;
};

int __attribute__((always_inline)) unlink_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->unlink.dentry, EVENT_UNLINK);
}

int __attribute__((always_inline)) trace__sys_unlink(int flags) {
    struct syscall_cache_t syscall = {
        .type = EVENT_UNLINK,
        .policy = fetch_policy(EVENT_UNLINK),
        .unlink = {
            .flags = flags,
        }
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE0(unlink) {
    return trace__sys_unlink(0);
}

SYSCALL_KPROBE3(unlinkat, int, dirfd, const char*, filename, int, flags) {
    return trace__sys_unlink(flags);
}

#define VFS_UNLINK_POSITION2 1
#define VFS_UNLINK_POSITION3 2

SEC("kprobe/vfs_unlink")
int kprobe_vfs_unlink(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNLINK);
    if (!syscall)
        return 0;

    if (syscall->unlink.file.path_key.ino) {
        return 0;
    }

    u64 vfs_unlink_dentry_position;
    LOAD_CONSTANT("vfs_unlink_dentry_position", vfs_unlink_dentry_position);

    struct dentry *dentry = (struct dentry *) PT_REGS_PARM2(ctx);

    // prevent a verifier bug
    bpf_probe_read(&dentry, sizeof(dentry), &dentry);

    // change the register based on the value of vfs_unlink_dentry_position
    if (vfs_unlink_dentry_position == VFS_UNLINK_POSITION3) {
        dentry = (struct dentry *) PT_REGS_PARM3(ctx);
    }

    // we resolve all the information before the file is actually removed
    syscall->unlink.dentry = dentry;
    set_file_inode(dentry, &syscall->unlink.file, 1);
    fill_file_metadata(dentry, &syscall->unlink.file.metadata);

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

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

SEC("kprobe/dr_unlink_callback")
int __attribute__((always_inline)) kprobe_dr_unlink_callback(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNLINK);
    if (!syscall)
        return 0;

    if (syscall->resolver.ret < 0) {
        return mark_as_discarded(syscall);
    }

    return 0;
}

int __attribute__((always_inline)) sys_unlink_ret(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_UNLINK);
    if (!syscall)
        return 0;

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
                .file = syscall->unlink.file,
            };

            struct proc_cache_t *entry = fill_process_context(&event.process);
            fill_container_context(entry, &event.container);
            fill_span_context(&event.span);

            send_event(ctx, EVENT_RMDIR, event);
        } else {
            struct unlink_event_t event = {
                .syscall.retval = retval,
                .file = syscall->unlink.file,
                .flags = syscall->unlink.flags,
            };

            struct proc_cache_t *entry = fill_process_context(&event.process);
            fill_container_context(entry, &event.container);
            fill_span_context(&event.span);

            send_event(ctx, EVENT_UNLINK, event);
        }
    }

    if (retval >= 0) {
        invalidate_inode(ctx, syscall->unlink.file.path_key.mount_id, syscall->unlink.file.path_key.ino, !pass_to_userspace);
    }

    return 0;
}

int __attribute__((always_inline)) kprobe_sys_unlink_ret(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return sys_unlink_ret(ctx, retval);
}

SEC("tracepoint/syscalls/sys_exit_unlink")
int tracepoint_syscalls_sys_exit_unlink(struct tracepoint_syscalls_sys_exit_t *args) {
    return sys_unlink_ret(args, args->ret);
}

SYSCALL_KRETPROBE(unlink) {
    return kprobe_sys_unlink_ret(ctx);
}

SEC("tracepoint/syscalls/sys_exit_unlinkat")
int tracepoint_syscalls_sys_exit_unlinkat(struct tracepoint_syscalls_sys_exit_t *args) {
    return sys_unlink_ret(args, args->ret);
}

SYSCALL_KRETPROBE(unlinkat) {
    return kprobe_sys_unlink_ret(ctx);
}

SEC("tracepoint/handle_sys_unlink_exit")
int tracepoint_handle_sys_unlink_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_unlink_ret(args, args->ret);
}

#endif
