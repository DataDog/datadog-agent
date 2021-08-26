#ifndef _LINK_H_
#define _LINK_H_

#include "syscalls.h"

struct link_event_t {
    struct kevent_t event;
    struct process_context_t process;
    struct container_context_t container;
    struct syscall_t syscall;
    struct file_t source;
    struct file_t target;
};

int __attribute__((always_inline)) link_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->link.src_dentry, EVENT_LINK) ||
           basename_approver(syscall, syscall->link.target_dentry, EVENT_LINK);
}

int __attribute__((always_inline)) trace__sys_link() {
    struct policy_t policy = fetch_policy(EVENT_LINK);
    if (is_discarded_by_process(policy.mode, EVENT_LINK)) {
        return 0;
    }

    struct syscall_cache_t syscall = {
        .type = EVENT_LINK,
        .policy = policy,
    };

    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE0(link) {
    return trace__sys_link();
}

SYSCALL_KPROBE0(linkat) {
    return trace__sys_link();
}

SEC("kprobe/vfs_link")
int kprobe__vfs_link(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_LINK);
    if (!syscall)
        return 0;

    if (syscall->link.target_dentry) {
        return 0;
    }

    struct dentry *src_dentry = (struct dentry *)PT_REGS_PARM1(ctx);

    syscall->link.src_dentry = src_dentry;
    syscall->link.target_dentry = (struct dentry *)PT_REGS_PARM3(ctx);
    if (filter_syscall(syscall, link_approvers)) {
        return discard_syscall(syscall);
    }

    fill_file_metadata(src_dentry, &syscall->link.src_file.metadata);
    syscall->link.target_file.metadata = syscall->link.src_file.metadata;

    // this is a hard link, source and target dentries are on the same filesystem & mount point
    // target_path was set by kprobe/filename_create before we reach this point.
    syscall->link.src_file.path_key.mount_id = get_path_mount_id(syscall->link.target_path);
    set_file_inode(src_dentry, &syscall->link.src_file, 0);

    // we generate a fake target key as the inode is the same
    syscall->link.target_file.path_key.ino = FAKE_INODE_MSW<<32 | bpf_get_prandom_u32();
    syscall->link.target_file.path_key.mount_id = syscall->link.src_file.path_key.mount_id;
    if (is_overlayfs(src_dentry))
        syscall->link.target_file.flags |= UPPER_LAYER;

    syscall->resolver.dentry = src_dentry;
    syscall->resolver.key = syscall->link.src_file.path_key;
    syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? EVENT_LINK : 0;
    syscall->resolver.callback = DR_LINK_SRC_CALLBACK_KPROBE_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

SEC("kprobe/dr_link_src_callback")
int __attribute__((always_inline)) dr_link_src_callback(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(EVENT_LINK);
    if (!syscall)
        return 0;

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        return discard_syscall(syscall);
    }

    return 0;
}

int __attribute__((always_inline)) sys_link_ret(void *ctx, int retval, int dr_type) {
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct syscall_cache_t *syscall = peek_syscall(EVENT_LINK);
    if (!syscall)
        return 0;

    syscall->resolver.dentry = syscall->link.target_dentry;
    syscall->resolver.key = syscall->link.target_file.path_key;
    syscall->resolver.discarder_type = 0;
    syscall->resolver.callback = dr_type == DR_KPROBE ? DR_LINK_DST_CALLBACK_KPROBE_KEY : DR_LINK_DST_CALLBACK_TRACEPOINT_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, dr_type);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall(EVENT_LINK);
    return 0;
}

int __attribute__((always_inline)) kprobe_sys_link_ret(struct pt_regs *ctx) {
    int retval = PT_REGS_RC(ctx);
    return sys_link_ret(ctx, retval, DR_KPROBE);
}

SEC("tracepoint/syscalls/sys_exit_link")
int tracepoint_syscalls_sys_exit_link(struct tracepoint_syscalls_sys_exit_t *args) {
    return sys_link_ret(args, args->ret, DR_TRACEPOINT);
}

SYSCALL_KRETPROBE(link) {
    return kprobe_sys_link_ret(ctx);
}

SEC("tracepoint/syscalls/sys_exit_linkat")
int tracepoint_syscalls_sys_exit_linkat(struct tracepoint_syscalls_sys_exit_t *args) {
    return sys_link_ret(args, args->ret, DR_TRACEPOINT);
}

SYSCALL_KRETPROBE(linkat) {
    return kprobe_sys_link_ret(ctx);
}

SEC("tracepoint/handle_sys_link_exit")
int tracepoint_handle_sys_link_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    return sys_link_ret(args, args->ret, DR_TRACEPOINT);
}

int __attribute__((always_inline)) dr_link_dst_callback(void *ctx, int retval) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_LINK);
    if (!syscall)
        return 0;

    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct link_event_t event = {
        .event.type = EVENT_LINK,
        .event.timestamp = bpf_ktime_get_ns(),
        .syscall.retval = retval,
        .source = syscall->link.src_file,
        .target = syscall->link.target_file,
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);

    send_event(ctx, EVENT_LINK, event);

    return 0;
}

SEC("kprobe/dr_link_dst_callback")
int __attribute__((always_inline)) kprobe_dr_link_dst_callback(struct pt_regs *ctx) {
    int ret = PT_REGS_RC(ctx);
    return dr_link_dst_callback(ctx, ret);
}

SEC("tracepoint/dr_link_dst_callback")
int __attribute__((always_inline)) tracepoint_dr_link_dst_callback(struct tracepoint_syscalls_sys_exit_t *args) {
    return dr_link_dst_callback(args, args->ret);
}

#endif
