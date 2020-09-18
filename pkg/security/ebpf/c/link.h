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
        .type = EVENT_LINK,
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
        .event.type = EVENT_LINK,
        .syscall = {
            .retval = retval,
            .timestamp = bpf_ktime_get_ns(),
        },
        .source = {
            .inode = syscall->link.src_key.ino,
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
