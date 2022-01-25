#ifndef _SETATTR_H_
#define _SETATTR_H_

#include "syscalls.h"

int __attribute__((always_inline)) chmod_approvers(struct syscall_cache_t *syscall);
int __attribute__((always_inline)) chown_approvers(struct syscall_cache_t *syscall);
int __attribute__((always_inline)) utime_approvers(struct syscall_cache_t *syscall);

int __attribute__((always_inline)) security_inode_predicate(u64 type) {
    return type == EVENT_UTIME || type == EVENT_CHMOD || type == EVENT_CHOWN;
}

SEC("kprobe/security_inode_setattr")
int kprobe_security_inode_setattr(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(security_inode_predicate);
    if (!syscall) {
        return 0;
    }

    struct dentry *dentry = (struct dentry *)PT_REGS_PARM1(ctx);
    fill_file_metadata(dentry, &syscall->setattr.file.metadata);

    struct iattr *iattr = (struct iattr *)PT_REGS_PARM2(ctx);
    if (iattr != NULL) {
        int valid;
        bpf_probe_read(&valid, sizeof(valid), &iattr->ia_valid);
        if (valid & ATTR_GID) {
            bpf_probe_read(&syscall->setattr.group, sizeof(syscall->setattr.group), &iattr->ia_gid);
        }

        if (valid & (ATTR_TOUCH | ATTR_ATIME_SET | ATTR_MTIME_SET)) {
            if (syscall->setattr.file.path_key.ino) {
                return 0;
            }
            bpf_probe_read(&syscall->setattr.atime, sizeof(syscall->setattr.atime), &iattr->ia_atime);
            bpf_probe_read(&syscall->setattr.mtime, sizeof(syscall->setattr.mtime), &iattr->ia_mtime);
        }
    }

    if (syscall->setattr.file.path_key.ino) {
        return 0;
    }

    syscall->setattr.dentry = dentry;

    // the mount id of path_key is resolved by kprobe/mnt_want_write. It is already set by the time we reach this probe.
    set_file_inode(dentry, &syscall->setattr.file, 0);

    u64 event_type = 0;
    switch (syscall->type) {
        case EVENT_UTIME:
            if (filter_syscall(syscall, utime_approvers)) {
                return discard_syscall(syscall);
            }
            event_type = EVENT_UTIME;
            break;
        case EVENT_CHMOD:
            if (filter_syscall(syscall, chmod_approvers)) {
                return discard_syscall(syscall);
            }
            event_type = EVENT_CHMOD;
            break;
        case EVENT_CHOWN:
            if (filter_syscall(syscall, chown_approvers)) {
                return discard_syscall(syscall);
            }
            event_type = EVENT_CHOWN;
            break;
    }

    syscall->resolver.dentry = syscall->setattr.dentry;
    syscall->resolver.key = syscall->setattr.file.path_key;
    syscall->resolver.discarder_type = syscall->policy.mode != NO_FILTER ? event_type : 0;
    syscall->resolver.callback = DR_SETATTR_CALLBACK_KPROBE_KEY;
    syscall->resolver.iteration = 0;
    syscall->resolver.ret = 0;

    resolve_dentry(ctx, DR_KPROBE);
    return 0;
}

SEC("kprobe/dr_setattr_callback")
int __attribute__((always_inline)) kprobe_dr_setattr_callback(struct pt_regs *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(security_inode_predicate);
    if (!syscall) {
        return 0;
    }

    if (syscall->resolver.ret == DENTRY_DISCARDED) {
        monitor_discarded(syscall->type);
        return discard_syscall(syscall);
    }

    return 0;
}

#endif
