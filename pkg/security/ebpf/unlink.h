#ifndef _UNLINK_H_
#define _UNLINK_H_

#include "syscalls.h"
#include "process.h"

struct unlink_event_t {
    struct event_t event;
    struct process_data_t process;
    unsigned long inode;
    dev_t         dev;
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
    struct dentry *dentry = (struct dentry *) PT_REGS_PARM2(ctx);
    struct path_key_t path_key = get_dentry_key(dentry);
    syscall->unlink.path_key = path_key;
    resolve_dentry(dentry, path_key);

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
        .dev = syscall->unlink.path_key.dev,
        .inode = syscall->unlink.path_key.ino,
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