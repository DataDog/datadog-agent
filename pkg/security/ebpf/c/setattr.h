#ifndef _SETATTR_H_
#define _SETATTR_H_

#include "syscalls.h"

SEC("kprobe/security_inode_setattr")
int kprobe__security_inode_setattr(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall(SYSCALL_UTIME | SYSCALL_CHMOD | SYSCALL_CHOWN);
    if (!syscall)
        return 0;

    struct dentry *dentry = (struct dentry *)PT_REGS_PARM1(ctx);

    struct iattr *iattr = (struct iattr *)PT_REGS_PARM2(ctx);
    if (iattr != NULL) {
        int valid;
        bpf_probe_read(&valid, sizeof(valid), &iattr->ia_valid);
        if (valid & ATTR_GID) {
            bpf_probe_read(&syscall->setattr.group, sizeof(syscall->setattr.group), &iattr->ia_gid);
        }

        if (valid & (ATTR_TOUCH | ATTR_ATIME_SET | ATTR_MTIME_SET)) {
            if (syscall->setattr.dentry) {
                syscall->setattr.real_inode = get_dentry_ino(dentry);
                return 0;
            }
            bpf_probe_read(&syscall->setattr.atime, sizeof(syscall->setattr.atime), &iattr->ia_atime);
            bpf_probe_read(&syscall->setattr.mtime, sizeof(syscall->setattr.mtime), &iattr->ia_mtime);
        }
    }

    // if second pass, ex: overlayfs, just cache the inode that will be used in ret
    if (syscall->setattr.dentry) {
        syscall->setattr.real_inode = get_dentry_ino(dentry);
        return 0;
    }

    syscall->setattr.dentry = dentry;

    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    syscall->setattr.path_key.ino = get_dentry_ino(syscall->setattr.dentry);
    syscall->setattr.path_key.path_id = bpf_get_prandom_u32();

    u64 event_type = 0;
    switch (syscall->type) {
        case SYSCALL_UTIME:
            event_type = EVENT_UTIME;
            break;
        case SYSCALL_CHMOD:
            event_type = EVENT_CHMOD;
            break;
        case SYSCALL_CHOWN:
            event_type = EVENT_CHOWN;
            break;
    }

    int ret = resolve_dentry(syscall->setattr.dentry, syscall->setattr.path_key, syscall->policy.mode != NO_FILTER ? event_type : 0);
    if (ret == DENTRY_DISCARDED) {
        pop_syscall(syscall->type);
    }

    return 0;
}

#endif
