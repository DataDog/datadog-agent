#ifndef _UNLINK_H_
#define _UNLINK_H_

#include "syscalls.h"
#include "process.h"

struct unlink_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    u32 flags;
    u32 discarder_revision;
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

SEC("kprobe/vfs_unlink")
int kprobe__vfs_unlink(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNLINK);
    if (!syscall)
        return 0;

    if (syscall->unlink.file.path_key.ino) {
        return 0;
    }

    // we resolve all the information before the file is actually removed
    struct dentry *dentry = (struct dentry *) PT_REGS_PARM2(ctx);
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
int __attribute__((always_inline)) dr_unlink_callback(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_UNLINK);
    if (!syscall)
        return 0;

    if (syscall->resolver.ret < 0) {
        return mark_as_discarded(syscall);
    }

    return 0;
}

int __attribute__((always_inline)) sys_unlink_ret(void *ctx, int retval) {
    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct syscall_cache_t *syscall = pop_syscall(EVENT_UNLINK);
    if (!syscall)
        return 0;

    // ensure that we invalidate all the layers
    u64 inode = syscall->unlink.file.path_key.ino;
    invalidate_inode(ctx, syscall->unlink.file.path_key.mount_id, inode, 1);

    u64 enabled_events = get_enabled_events();
    int pass_to_userspace = !syscall->discarded &&
                            (mask_has_event(enabled_events, EVENT_UNLINK) ||
                             mask_has_event(enabled_events, EVENT_RMDIR));
    if (pass_to_userspace) {
        struct unlink_event_t event = {
            .syscall.retval = retval,
            .file = syscall->unlink.file,
            .flags = syscall->unlink.flags,
            .discarder_revision = get_discarder_revision(syscall->unlink.file.path_key.mount_id),
        };

        struct proc_cache_t *entry = fill_process_context(&event.process);
        fill_container_context(entry, &event.container);

        send_event(ctx, syscall->unlink.flags&AT_REMOVEDIR ? EVENT_RMDIR : EVENT_UNLINK, event);
    }

    invalidate_inode(ctx, syscall->unlink.file.path_key.mount_id, syscall->unlink.file.path_key.ino, !pass_to_userspace);

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

#endif
