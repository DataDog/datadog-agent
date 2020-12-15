#ifndef _OVERLAYFS_H_
#define _OVERLAYFS_H_

#include "syscalls.h"

#define set_path_key_inode(dentry, path_key, invalidate) \
    path_key.path_id = get_path_id(invalidate); \
    if (!path_key.ino) path_key.ino = get_dentry_ino(dentry); \
    u64 lower_inode = get_ovl_lower_ino(dentry); \
    u64 upper_inode = get_ovl_upper_ino(dentry); \
    if (lower_inode) path_key.ino = lower_inode; \
    else if (upper_inode) path_key.ino = upper_inode

SEC("kprobe/ovl_want_write")
int kprobe__ovl_want_write(struct pt_regs *ctx) {
   struct syscall_cache_t *syscall = peek_syscall(SYSCALL_OPEN | SYSCALL_EXEC | SYSCALL_UNLINK);
    if (!syscall)
        return 0;

    return 0;
}   

SEC("kretprobe/ovl_d_real")
int kretprobe__ovl_d_real(struct pt_regs *ctx) {
   struct syscall_cache_t *syscall = peek_syscall(SYSCALL_OPEN | SYSCALL_EXEC | SYSCALL_UNLINK);
    if (!syscall)
        return 0;

    return 0;
}

SEC("kretprobe/ovl_dentry_lower")
int kprobe__ovl_dentry_lower(struct pt_regs *ctx) {
   struct syscall_cache_t *syscall = peek_syscall(SYSCALL_OPEN | SYSCALL_EXEC | SYSCALL_UNLINK);
    if (!syscall)
        return 0;

    return 0;
}

SEC("kretprobe/ovl_dentry_upper")
int kprobe__ovl_dentry_upper(struct pt_regs *ctx) {
   struct syscall_cache_t *syscall = peek_syscall(SYSCALL_OPEN | SYSCALL_EXEC | SYSCALL_UNLINK);
    if (!syscall)
        return 0;

    return 0;
}

#endif