#ifndef _MKDIR_H_
#define _MKDIR_H_

#include "syscalls.h"

struct mkdir_event_t {
    struct event_t event;
    struct process_data_t process;
    long   inode;
    int    mount_id;
    int    mode;
};

int __attribute__((always_inline)) trace__sys_mkdir(struct pt_regs *ctx) {
    if (filter_process())
        return 0;

    struct syscall_cache_t syscall = {
        .mkdir = {
            .mode = (umode_t) PT_REGS_PARM2(ctx)
        }
    };

    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/__x64_sys_mkdir")
int kprobe__sys_mkdir(struct pt_regs *ctx) {
    return trace__sys_mkdir(ctx);
}

SEC("kprobe/__x64_sys_mkdirat")
int kprobe__sys_mkdirat(struct pt_regs *ctx) {
    return trace__sys_mkdir(ctx);
}

SEC("kprobe/vfs_mkdir")
int kprobe__vfs_mkdir(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    syscall->mkdir.dir = (struct inode *)PT_REGS_PARM1(ctx);
    syscall->mkdir.dentry = (struct dentry *)PT_REGS_PARM2(ctx);

    return 0;
}

int __attribute__((always_inline)) trace__sys_mkdir_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct mkdir_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_VFS_MKDIR,
        .event.timestamp = bpf_ktime_get_ns(),
        .inode = get_dentry_ino(syscall->mkdir.dentry),
        .mount_id = get_inode_mount_id(syscall->mkdir.dir),
        .mode = syscall->mkdir.mode,
    };

    fill_process_data(&event.process);
    resolve_dentry(syscall->mkdir.dentry, event.inode);

    send_event(ctx, event);

    return 0;
}

SEC("kretprobe/__x64_sys_mkdir")
int kretprobe__sys_mkdir(struct pt_regs *ctx) {
    return trace__sys_mkdir_ret(ctx);
}

SEC("kretprobe/__x64_sys_mkdirat")
int kretprobe__sys_mkdirat(struct pt_regs *ctx) {
    return trace__sys_mkdir_ret(ctx);
}

#endif