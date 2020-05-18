#ifndef _UNLINK_H_
#define _UNLINK_H_

#include "syscalls.h"
#include "process.h"

struct unlink_event_t {
    struct event_t event;
    struct process_data_t process;
    long   inode;
    u32    pathname_key;
    int    mount_id;
};

int trace__sys_unlink() {
    if (filter_process())
        return 0;

    struct syscall_cache_t syscall = {};
    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/sys_unlink")
int kprobe__sys_unlink(struct pt_regs *ctx) {
    return trace__sys_unlink();
}

SEC("kprobe/sys_unlinkat")
int kprobe__sys_unlinkat(struct pt_regs *ctx) {
    return trace__sys_unlink();
}

SEC("kprobe/vfs_unlink")
int kprobe__vfs_unlink(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    // we resolve all the information before the file is actually removed
    syscall->unlink.mount_id = get_inode_mount_id((struct inode *) PT_REGS_PARM1(ctx));
    struct dentry *dentry = (struct dentry *) PT_REGS_PARM2(ctx);
    syscall->unlink.inode = get_dentry_ino(dentry);
    resolve_dentry(dentry, syscall->unlink.inode);

    return 0;
}

int __attribute__((always_inline)) trace__sys_unlink_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct unlink_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_VFS_UNLINK,
        .event.timestamp = bpf_ktime_get_ns(),
        .inode = syscall->unlink.inode,
        .mount_id = syscall->unlink.mount_id,
    };

    fill_process_data(&event.process);

    send_event(ctx, event);

    return 0;
}

SEC("kretprobe/sys_unlink")
int kretprobe__sys_unlink(struct pt_regs *ctx) {
    return trace__sys_unlink_ret(ctx);
}

SEC("kretprobe/sys_unlinkat")
int kretprobe__sys_unlinkat(struct pt_regs *ctx) {
    return trace__sys_unlink_ret(ctx);
}

#endif