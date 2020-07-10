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
        syscall->setattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_CHMOD:
        syscall->setattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_CHOWN:
        syscall->setattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_RENAME:
        syscall->rename.src_key.mount_id = get_vfsmount_mount_id(mnt);
        syscall->rename.target_key.mount_id = syscall->rename.src_key.mount_id;
        break;
    case EVENT_RMDIR:
        syscall->rmdir.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    case EVENT_UNLINK:
        syscall->unlink.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    }
    return 0;
}

SEC("kprobe/mnt_want_write_file")
int kprobe__mnt_want_write_file(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall();
    if (!syscall)
        return 0;

    struct file *file = (struct file *)PT_REGS_PARM1(ctx);
    struct vfsmount *mnt;
    bpf_probe_read(&mnt, sizeof(mnt), &file->f_path.mnt);

    switch (syscall->type) {
    case EVENT_CHOWN:
        syscall->setattr.path_key.mount_id = get_vfsmount_mount_id(mnt);
        break;
    }
    return 0;
}

#endif
