#ifndef _RMDIR_H_
#define _RMDIR_H_

#include "syscalls.h"

struct rmdir_event_t {
    struct event_t event;
    struct process_data_t process;
    unsigned long inode;
    dev_t         dev;
    u32           padding;
};

SYSCALL_KPROBE(rmdir) {
    if (filter_process())
        return 0;

    struct syscall_cache_t syscall = {};
    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/vfs_rmdir")
int kprobe__vfs_rmdir(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    // we resolve all the information before the file is actually removed
    struct dentry *dentry = (struct dentry *) PT_REGS_PARM2(ctx);
    struct path_key_t path_key = get_dentry_key(dentry);
    syscall->rmdir.path_key = path_key;
    resolve_dentry(dentry, path_key);

    return 0;
}

SYSCALL_KRETPROBE(rmdir) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct rmdir_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_VFS_RMDIR,
        .event.timestamp = bpf_ktime_get_ns(),
        .inode = syscall->rmdir.path_key.ino,
        .dev = syscall->rmdir.path_key.dev,
    };

    fill_process_data(&event.process);

    send_event(ctx, event);

    return 0;
}

#endif