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
        .type = SYSCALL_LINK,
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
    struct syscall_cache_t *syscall = peek_syscall(SYSCALL_LINK);
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

    syscall->link.src_overlay_numlower = get_overlay_numlower(src_dentry);

    // this is a hard link, source and target dentries are on the same filesystem & mount point
    // target_path was set by kprobe/filename_create before we reach this point.
    syscall->link.src_key.mount_id = get_path_mount_id(syscall->link.target_path);
    set_path_key_inode(src_dentry, &syscall->link.src_key, 1);

    // we generate a fake target key as the inode is the same
    syscall->link.target_key.ino = FAKE_INODE_MSW<<32 | bpf_get_prandom_u32();
    syscall->link.target_key.mount_id = syscall->link.src_key.mount_id;

    int ret = resolve_dentry(src_dentry, syscall->link.src_key, syscall->policy.mode != NO_FILTER ? EVENT_LINK : 0);
    if (ret == DENTRY_DISCARDED) {
        return discard_syscall(syscall);
    }

    return 0;
}

int __attribute__((always_inline)) trace__sys_link_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_LINK);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct link_event_t event = {
        .event.type = EVENT_LINK,
        .event.timestamp = bpf_ktime_get_ns(),
        .syscall.retval = retval,
        .source = {
            .inode = syscall->link.src_key.ino,
            .mount_id = syscall->link.src_key.mount_id,
            .overlay_numlower = syscall->link.src_overlay_numlower,
            .path_id = syscall->link.src_key.path_id,
        },
        .target = {
            .inode = syscall->link.target_key.ino,
            .mount_id = syscall->link.target_key.mount_id,
            .overlay_numlower = get_overlay_numlower(syscall->link.target_dentry),
        }
    };

    struct proc_cache_t *entry = fill_process_context(&event.process);
    fill_container_context(entry, &event.container);

    resolve_dentry(syscall->link.target_dentry, syscall->link.target_key, 0);

    send_event(ctx, EVENT_LINK, event);

    return 0;
}

SYSCALL_KRETPROBE(link) {
    return trace__sys_link_ret(ctx);
}

SYSCALL_KRETPROBE(linkat) {
    return trace__sys_link_ret(ctx);
}

#endif
