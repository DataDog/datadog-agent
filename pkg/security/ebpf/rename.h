#ifndef _RENAME_H_
#define _RENAME_H_

#include "syscalls.h"

struct rename_event_t {
    struct event_t event;
    struct process_data_t process;
    unsigned long src_inode;
    int           src_mount_id;
    u32           padding;
    unsigned long target_inode;
    int           target_mount_id;
    u32           padding2;
};

int __attribute__((always_inline)) trace__sys_rename() {
    if (filter_process())
        return 0;

    struct syscall_cache_t syscall = {};
    cache_syscall(&syscall);

    return 0;
}

SEC("kprobe/sys_rename")
int kprobe__sys_rename(struct pt_regs *ctx) {
    return trace__sys_rename();
}

SEC("kprobe/sys_renameat")
int kprobe__sys_renameat(struct pt_regs *ctx) {
    return trace__sys_rename();
}

SEC("kprobe/sys_renameat2")
int kprobe__sys_renameat2(struct pt_regs *ctx) {
    return trace__sys_rename();
}

SEC("kprobe/vfs_rename")
int kprobe__vfs_rename(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    syscall->rename.src_dir = (struct inode *)PT_REGS_PARM1(ctx);
    syscall->rename.src_dentry = (struct dentry *)PT_REGS_PARM2(ctx);
    syscall->rename.target_dir = (struct inode *)PT_REGS_PARM3(ctx);
    syscall->rename.target_dentry = (struct dentry *)PT_REGS_PARM4(ctx);

    // we generate a fake source inode as the inode is (can be ?) reused
    syscall->rename.random_inode = bpf_get_prandom_u32() << 32 | bpf_get_prandom_u32();
    resolve_dentry(syscall->rename.src_dentry, syscall->rename.random_inode);

    return 0;
}

int __attribute__((always_inline)) trace__sys_rename_ret(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = pop_syscall();
    if (!syscall)
        return 0;

    struct rename_event_t event = {
        .event.retval = PT_REGS_RC(ctx),
        .event.type = EVENT_VFS_RENAME,
        .event.timestamp = bpf_ktime_get_ns(),
        .src_mount_id = get_inode_mount_id(syscall->rename.src_dir),
        .src_inode = syscall->rename.random_inode,
        .target_mount_id = get_inode_mount_id(syscall->rename.src_dir),
        .target_inode = get_dentry_ino(syscall->rename.src_dentry),
    };

    fill_process_data(&event.process);
    resolve_dentry(syscall->rename.target_dentry, event.target_inode);

    send_event(ctx, event);

    return 0;
}

SEC("kretprobe/sys_rename")
int kretprobe__sys_rename(struct pt_regs *ctx) {
    return trace__sys_rename_ret(ctx);

}

SEC("kretprobe/sys_renameat")
int kretprobe__sys_renameat(struct pt_regs *ctx) {
    return trace__sys_rename_ret(ctx);
}

SEC("kretprobe/sys_renameat2")
int kretprobe__sys_renameat2(struct pt_regs *ctx) {
    return trace__sys_rename_ret(ctx);
}

#endif