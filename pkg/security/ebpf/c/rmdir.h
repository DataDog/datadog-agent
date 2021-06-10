#ifndef _RMDIR_H_
#define _RMDIR_H_

#include "syscalls.h"

struct rmdir_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    u32 discarder_revision;
    u32 padding;
};

int __attribute__((always_inline)) rmdir_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->rmdir.dentry, EVENT_RMDIR);
}
int __attribute__((always_inline)) unlink_approvers(struct syscall_cache_t *syscall);

SYSCALL_KPROBE0(rmdir) {
    struct syscall_cache_t syscall = {
        .type = EVENT_RMDIR,
        .policy = fetch_policy(EVENT_RMDIR),
    };

    cache_syscall(&syscall);

    return 0;
}

int __attribute__((always_inline)) rmdir_predicate(u64 type) {
    return type == EVENT_RMDIR || type == EVENT_UNLINK;
}

// security_inode_rmdir is shared between rmdir and unlink syscalls
SEC("kprobe/security_inode_rmdir")
int kprobe__security_inode_rmdir(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(rmdir_predicate);
    if (!syscall)
        return 0;

    struct path_key_t key = {};
    struct dentry *dentry = NULL;

    switch (syscall->type) {
        case EVENT_RMDIR:
            if (syscall->rmdir.file.path_key.ino) {
                return 0;
            }

            // we resolve all the information before the file is actually removed
            dentry = (struct dentry *)PT_REGS_PARM2(ctx);
            set_file_inode(dentry, &syscall->rmdir.file, 1);
            fill_file_metadata(dentry, &syscall->rmdir.file.metadata);

            // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
            key = syscall->rmdir.file.path_key;

            syscall->rmdir.dentry = dentry;
            if (filter_syscall(syscall, rmdir_approvers)) {
                return mark_as_discarded(syscall);
            }

            break;
        case EVENT_UNLINK:
            if (syscall->unlink.file.path_key.ino) {
                return 0;
            }

            // we resolve all the information before the file is actually removed
            dentry = (struct dentry *) PT_REGS_PARM2(ctx);
            set_file_inode(dentry, &syscall->unlink.file, 1);
            fill_file_metadata(dentry, &syscall->unlink.file.metadata);

            // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
            key = syscall->unlink.file.path_key;

            syscall->unlink.dentry = dentry;
            syscall->policy = fetch_policy(EVENT_RMDIR);
            if (filter_syscall(syscall, rmdir_approvers)) {
                return mark_as_discarded(syscall);
            }

            break;
        default:
            return 0;
    }

    if (is_discarded_by_process(syscall->policy.mode, syscall->type)) {
        return mark_as_discarded(syscall);
    }

    if (dentry != NULL) {
        syscall->resolver.key = key;
        syscall->resolver.dentry = dentry;
        syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? syscall->type : 0;
        syscall->resolver.callback = DR_SECURITY_INODE_RMDIR_CALLBACK_KPROBE_KEY;
        syscall->resolver.iteration = 0;
        syscall->resolver.ret = 0;

        resolve_dentry(ctx, DR_KPROBE);
    }
    return 0;
}

SEC("kprobe/dr_security_inode_rmdir_callback")
int __attribute__((always_inline)) dr_security_inode_rmdir_callback(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(rmdir_predicate);
    if (!syscall)
        return 0;

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        return mark_as_discarded(syscall);
    }
    return 0;
}

int __attribute__((always_inline)) sys_rmdir_ret(void *ctx, int retval) {
    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    struct syscall_cache_t *syscall = pop_syscall_with(rmdir_predicate);
    if (!syscall)
        return 0;

    int pass_to_userspace = !syscall->discarded && is_event_enabled(EVENT_RMDIR);
    if (pass_to_userspace) {
        struct rmdir_event_t event = {
            .syscall.retval = retval,
            .file = syscall->rmdir.file,
            .discarder_revision = get_discarder_revision(syscall->rmdir.file.path_key.mount_id),
        };

        struct proc_cache_t *entry = fill_process_context(&event.process);
        fill_container_context(entry, &event.container);

        send_event(ctx, EVENT_RMDIR, event);
    }

    invalidate_inode(ctx, syscall->rmdir.file.path_key.mount_id, syscall->rmdir.file.path_key.ino, !pass_to_userspace);

    return 0;
}

SEC("tracepoint/syscalls/sys_exit_rmdir")
int tracepoint_syscalls_sys_exit_rmdir(struct tracepoint_syscalls_sys_exit_t *args) {
    return sys_rmdir_ret(args, args->ret);
}

SYSCALL_KRETPROBE(rmdir) {
    int retval = PT_REGS_RC(ctx);
    return sys_rmdir_ret(ctx, retval);
}

#endif
