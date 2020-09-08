#ifndef _MNT_H_
#define _MNT_H_

#include "syscalls.h"

SEC("kprobe/mnt_want_write")
int kprobe__mnt_want_write(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    struct vfsmount *mnt = (struct vfsmount *)PT_REGS_PARM1(ctx);

    switch (syscall->type) {
    case EVENT_UTIME:
        if (syscall->setattr.path_key.mount_id > 0)
            return 0;
        syscall->setattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_CHMOD:
        if (syscall->setattr.path_key.mount_id > 0)
            return 0;
        syscall->setattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_CHOWN:
        if (syscall->setattr.path_key.mount_id > 0)
            return 0;
        syscall->setattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_RENAME:
        if (syscall->rename.src_key.mount_id > 0)
            return 0;
        syscall->rename.src_key.mount_id = get_vfsmount_mount_id(mnt);
        syscall->rename.target_key.mount_id = syscall->rename.src_key.mount_id;
        break;
    case EVENT_RMDIR:
        if (syscall->rmdir.path_key.mount_id > 0)
            return 0;
        syscall->rmdir.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_UNLINK:
        if (syscall->unlink.path_key.mount_id > 0)
            return 0;
        syscall->unlink.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_SETXATTR:
        if (syscall->setxattr.path_key.mount_id > 0)
            return 0;
        syscall->setxattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_REMOVEXATTR:
        if (syscall->setxattr.path_key.mount_id > 0)
            return 0;
        syscall->setxattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    }
    return 0;
}

int __attribute__((always_inline)) trace__mnt_want_write_file(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    struct file *file = (struct file *)PT_REGS_PARM1(ctx);
    struct vfsmount *mnt;
    bpf_probe_read(&mnt, sizeof(mnt), &file->f_path.mnt);

    switch (syscall->type) {
    case EVENT_CHOWN:
        if (syscall->setattr.path_key.mount_id > 0)
            return 0;
        syscall->setattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_SETXATTR:
        if (syscall->setxattr.path_key.mount_id > 0)
            return 0;
        syscall->setxattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_REMOVEXATTR:
        if (syscall->setxattr.path_key.mount_id > 0)
            return 0;
        syscall->setxattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    }
    return 0;
}

SEC("kprobe/mnt_want_write_file")
int kprobe__mnt_want_write_file(struct pt_regs *ctx) {
    return trace__mnt_want_write_file(ctx);
}

// mnt_want_write_file_path was used on old kernels (RHEL 7)
SEC("kprobe/mnt_want_write_file_path")
int kprobe__mnt_want_write_file_path(struct pt_regs *ctx) {
    return trace__mnt_want_write_file(ctx);
}

#endif
