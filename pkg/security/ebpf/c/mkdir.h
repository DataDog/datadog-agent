#ifndef _MKDIR_H_
#define _MKDIR_H_

#include "syscalls.h"

struct mkdir_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct span_context_t span;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t file;
    u32 mode;
    u32 padding;
};

int __attribute__((always_inline)) mkdir_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->mkdir.dentry, EVENT_MKDIR);
}

long __attribute__((always_inline)) trace__sys_mkdir(u8 async, umode_t mode) {
    struct policy_t policy = fetch_policy(EVENT_MKDIR);
    if (is_discarded_by_process(policy.mode, EVENT_MKDIR)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_MKDIR,
        .policy = policy,
        .async = async,
        .mkdir = {
            .mode = mode
        }
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE2(mkdir, const char*, filename, umode_t, mode)
{
    return trace__sys_mkdir(SYNC_SYSCALL, mode);
}

SYSCALL_KPROBE3(mkdirat, int, dirfd, const char*, filename, umode_t, mode)
{
    return trace__sys_mkdir(SYNC_SYSCALL, mode);
}

SEC("kprobe/vfs_mkdir")
int kprobe_vfs_mkdir(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MKDIR);
    if (!syscall) {
        return 0;
    }

    if (syscall->mkdir.dentry) {
        return 0;
    }

    syscall->mkdir.dentry = (struct dentry *) PT_REGS_PARM2(ctx);
    // change the register based on the value of vfs_mkdir_dentry_position
    if (get_vfs_mkdir_dentry_position() == VFS_ARG_POSITION3) {
        // prevent the verifier from whining
        bpf_probe_read(&syscall->mkdir.dentry, sizeof(syscall->mkdir.dentry), &syscall->mkdir.dentry);
        syscall->mkdir.dentry = (struct dentry *) PT_REGS_PARM3(ctx);
    }

    syscall->mkdir.file.path_key.mount_id = get_path_mount_id(syscall->mkdir.path);

    if (filter_syscall(syscall, mkdir_approvers)) {
        return discard_syscall(syscall);
    }

    return 0;
}

int __attribute__((always_inline)) sys_mkdir_ret(void *ctx, int retval, int dr_type) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MKDIR);
    if (!syscall) {
        return 0;
    }
    if (IS_UNHANDLED_ERROR(retval)) {
        discard_syscall(syscall);
        return 0;
    }

    // the inode of the dentry was not properly set when kprobe/security_path_mkdir was called, make sure we grab it now
    set_file_inode(syscall->mkdir.dentry, &syscall->mkdir.file, 0);

    syscall->resolver.key = syscall->mkdir.file.path_key;
    syscall->resolver.dentry = syscall->mkdir.dentry;
    syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? EVENT_MKDIR : 0;
    syscall->resolver.callback = dr_type == DR_KPROBE ? DR_MKDIR_CALLBACK_KPROBE_KEY : DR_MKDIR_CALLBACK_TRACEPOINT_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, dr_type);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_MKDIR);
    return 0;
}

SEC("kprobe/do_mkdirat")
int kprobe_do_mkdirat(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_MKDIR);
    if (!syscall) {
        umode_t mode = (umode_t)PT_REGS_PARM3(ctx);
        return trace__sys_mkdir(ASYNC_SYSCALL, mode);
    }
    return 0;
}

SEC("kretprobe/do_mkdirat")
int kretprobe_do_mkdirat(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return sys_mkdir_ret(ctx, retval, DR_KPROBE);
}

int __attribute__((always_inline)) kprobe_sys_mkdir_ret(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return sys_mkdir_ret(ctx, retval, DR_KPROBE);
}

SYSCALL_KRETPROBE(mkdir)
{
    return kprobe_sys_mkdir_ret(ctx);
}

SYSCALL_KRETPROBE(mkdirat) {
    return kprobe_sys_mkdir_ret(ctx);
}

SEC("tracepoint/handle_sys_mkdir_exit")
int tracepoint_handle_sys_mkdir_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_mkdir_ret(args, args->ret, DR_TRACEPOINT);
}

int __attribute__((always_inline)) dr_mkdir_callback(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_MKDIR);
    if (!syscall) {
        return 0;
    }

    if (IS_UNHANDLED_ERROR(retval)) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(EVENT_MKDIR);
        return 0;
    }

    struct mkdir_event_t event = {
        .syscall.retval = retval,
        .event.flags = syscall->async ? EVENT_FLAGS_ASYNC : 0,
        .file = syscall->mkdir.file,
        .mode = syscall->mkdir.mode,
    };

    fill_file_metadata(syscall->mkdir.dentry, &event.file.metadata);
    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);
    fill_span_context(&event.span);

    send_event(ctx, EVENT_MKDIR, event);
    return 0;
}

SEC("kprobe/dr_mkdir_callback")
int __attribute__((always_inline)) kprobe_dr_mkdir_callback(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return dr_mkdir_callback(ctx, retval);
}

SEC("tracepoint/dr_mkdir_callback")
int __attribute__((always_inline)) tracepoint_dr_mkdir_callback(struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_mkdir_callback(args, args->ret);
}

#endif
