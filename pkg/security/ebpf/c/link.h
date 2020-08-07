#ifndef _LINK_H_
#define _LINK_H_

#include "syscalls.h"

struct link_event_t {
    struct event_t event;
    struct process_data_t process;
    char container_id[CONTAINER_ID_LEN];
    int src_mount_id;
    u32 padding;
    unsigned long src_inode;
    unsigned long target_inode;
    int target_mount_id;
    int src_overlay_numlower;
    int target_overlay_numlower;
    u32 padding2;
};

int __attribute__((always_inline)) trace__sys_link() {
    struct syscall_cache_t syscall = {
        .type = EVENT_LINK,
    };
    cache_syscall(&syscall);

    return 0;
}

SYSCALL_KPROBE(link) {
    return trace__sys_link();
}

SYSCALL_KPROBE(linkat) {
    return trace__sys_link();
}

SEC("kprobe/vfs_link")
int kprobe__vfs_link(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;
    // In a container, vfs_link can be called multiple times to handle the different layers of the overlay filesystem.
    // The first call is the only one we really care about, the subsequent calls contain paths to the overlay work layer.
    if (syscall->link.target_dentry)
        return 0;

    struct dentry *dentry = (struct dentry *)PT_REGS_PARM1(ctx);
    syscall->link.target_dentry = (struct dentry *)PT_REGS_PARM3(ctx);
    syscall->link.src_overlay_numlower = get_overlay_numlower(dentry);
    // this is a hard link, source and target dentries are on the same filesystem & mount point
    // target_path was set by kprobe/filename_create before we reach this point.
    syscall->link.src_key = get_key(dentry, syscall->link.target_path);
    // we generate a fake target key as the inode is the same
    syscall->link.target_key.ino = bpf_get_prandom_u32() << 32 | bpf_get_prandom_u32();
    syscall->link.target_key.mount_id = syscall->link.src_key.mount_id;
    get_key(syscall->link.target_dentry, syscall->link.target_path);

    resolve_dentry(dentry, syscall->link.src_key, NULL);
    return 0;
}

int __attribute__((always_inline)) trace__sys_link_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    int retval = PT_REGS_RC(ctx);
    if (IS_UNHANDLED_ERROR(retval))
        return 0;

    struct link_event_t event = {
        .event.retval = retval,
        .event.type = EVENT_LINK,
        .event.timestamp = bpf_ktime_get_ns(),
        .src_inode = syscall->link.src_key.ino,
        .src_mount_id = syscall->link.src_key.mount_id,
        .target_inode = syscall->link.target_key.ino,
        .target_mount_id = syscall->link.target_key.mount_id,
        .src_overlay_numlower = syscall->link.src_overlay_numlower,
        .target_overlay_numlower = get_overlay_numlower(syscall->link.target_dentry),
    };

    fill_process_data(&event.process);
    resolve_dentry(syscall->link.target_dentry, syscall->link.target_key, NULL);

    // add process cache data
    struct proc_cache_t *entry = get_pid_cache(syscall->pid);
    if (entry) {
        copy_container_id(event.container_id, entry->container_id);
        event.process.numlower = entry->numlower;
    }

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
