#ifndef _RMDIR_H_
#define _RMDIR_H_

#include "syscalls.h"

struct rmdir_event_t {
    struct event_t event;
    struct process_data_t process;
    long   inode;
    int    mount_id;
};

SEC("kprobe/__x64_sys_rmdir")
int kprobe__sys_rmdir(struct pt_regs *ctx) {
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
    syscall->rmdir.mount_id = get_inode_mount_id((struct inode *) PT_REGS_PARM1(ctx));
    struct dentry *dentry = (struct dentry *) PT_REGS_PARM2(ctx);
    syscall->rmdir.inode = get_dentry_ino(dentry);
    resolve_dentry(dentry, syscall->rmdir.inode);

    return 0;
}

SEC("kretprobe/__x64_sys_rmdir")
int kretprobe__sys_rmdir(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct rmdir_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_VFS_RMDIR,
        .event.timestamp = bpf_ktime_get_ns(),
        .inode = syscall->rmdir.inode,
        .mount_id = syscall->rmdir.mount_id,
    };

    fill_process_data(&event.process);

    send_event(ctx, event);

    return 0;
}

#endif