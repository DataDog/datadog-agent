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
    set_file_inode(src_dentry, &syscall->link.src_file, 1);

    // we generate a fake target key as the inode is the same
    syscall->link.target_file.path_key.ino = FAKE_INODE_MSW<<32 | bpf_get_prandom_u32();
    syscall->link.target_file.path_key.mount_id = syscall->link.src_file.path_key.mount_id;
    if (is_overlayfs(src_dentry))
        syscall->link.target_file.flags |= UPPER_LAYER;

    int ret = resolve_dentry(src_dentry, syscall->link.src_file.path_key, syscall->policy.mode != NO_FILTER ? EVENT_LINK : 0);
    if (ret == DENTRY_DISCARDED) {
        return discard_syscall(syscall);
    }

    return 0;
}

int __attribute__((always_inline)) do_sys_link_ret(void *ctx, struct syscall_cache_t *syscall, int retval) {
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

    resolve_dentry(syscall->link.target_dentry, syscall->link.target_file.path_key, 0);

    send_event(ctx, EVENT_LINK, event);

    return 0;
}

SEC("tracepoint/handle_sys_link_exit")
int handle_sys_link_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_LINK);
    if (!syscall)
        return 0;

    return do_sys_link_ret(args, syscall, args->ret);
}

int __attribute__((always_inline)) trace__sys_link_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(EVENT_LINK);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    return do_sys_link_ret(ctx, syscall, retval);
}

SYSCALL_KRETPROBE(link) {
    return trace__sys_link_ret(ctx);
}

SYSCALL_KRETPROBE(linkat) {
    return trace__sys_link_ret(ctx);
}

#endif
