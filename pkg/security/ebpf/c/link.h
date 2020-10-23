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

int __attribute__((always_inline)) trace__sys_link() {
    struct syscall_cache_t syscall = {
        .type = SYSCALL_LINK,
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

    struct dentry *dentry = (struct dentry *)PT_REGS_PARM1(ctx);

    // if second pass, ex: overlayfs, just cache the inode that will be used in ret
    if (syscall->link.target_dentry) {
        syscall->link.real_src_inode = get_dentry_ino(dentry);
        return 0;
    }

    syscall->link.target_dentry = (struct dentry *)PT_REGS_PARM3(ctx);
    syscall->link.src_overlay_numlower = get_overlay_numlower(dentry);
    // this is a hard link, source and target dentries are on the same filesystem & mount point
    // target_path was set by kprobe/filename_create before we reach this point.
    syscall->link.src_key = get_key(dentry, syscall->link.target_path);
    // we generate a fake target key as the inode is the same
    syscall->link.target_key.ino = bpf_get_prandom_u32() << 32 | bpf_get_prandom_u32();
    syscall->link.target_key.mount_id = syscall->link.src_key.mount_id;

    resolve_dentry(dentry, syscall->link.src_key, NULL);

    return 0;
}

int __attribute__((always_inline)) trace__sys_link_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall(SYSCALL_LINK);
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    // add an real entry to reach the first dentry with the proper inode
    u64 inode = syscall->link.src_key.ino;
    if (syscall->link.real_src_inode) {
        inode = syscall->link.real_src_inode;
        link_dentry_inode(syscall->link.src_key, inode);
    }

    struct link_event_t event = {
        .event.type = EVENT_LINK,
        .syscall = {
            .retval = retval,
            .timestamp = bpf_ktime_get_ns(),
        },
        .source = {
            .inode = inode,
            .mount_id = syscall->link.src_key.mount_id,
            .overlay_numlower = syscall->link.src_overlay_numlower,
        },
        .target = {
            .inode = syscall->link.target_key.ino,
            .mount_id = syscall->link.target_key.mount_id,
            .overlay_numlower = get_overlay_numlower(syscall->link.target_dentry),
        }
    };

    struct proc_cache_t *entry = fill_process_data(&event.process);
    fill_container_data(entry, &event.container);

    resolve_dentry(syscall->link.target_dentry, syscall->link.target_key, NULL);

    send_event(ctx, event);

    return 0;
}

SYSCALL_KRETPROBE(link) {
    return trace__sys_link_ret(ctx);
}

SYSCALL_KRETPROBE(linkat) {
    return trace__sys_link_ret(ctx);
}

#endif
