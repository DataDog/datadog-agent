#ifndef _HOOKS_SETATTR_H_
#define _HOOKS_SETATTR_H_

#include "constants/syscall_macro.h"
#include "constants/offsets/filesystem.h"
#include "helpers/approvers.h"
#include "helpers/events_predicates.h"
#include "helpers/filesystem.h"
#include "helpers/syscalls.h"

HOOK_ENTRY("security_inode_setattr")
int hook_security_inode_setattr(ctx_t *ctx) {
    struct syscall_cache_t *syscall = peek_syscall_with(security_inode_predicate);
    if (!syscall) {
        return 0;
    }

    u64 param1 = CTX_PARM1(ctx);
    u64 param2 = CTX_PARM2(ctx);

    struct dentry *dentry;
    struct iattr *iattr;
    if (security_have_usernamespace_first_arg()) {
        dentry = (struct dentry *)param2;
        iattr = (struct iattr *)CTX_PARM3(ctx);
    } else {
        dentry = (struct dentry *)param1;
        iattr = (struct iattr *)param2;
    }

    fill_file(dentry, &syscall->setattr.file);

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

    if (is_non_mountable_dentry(dentry)) {
        pop_syscall_with(security_inode_predicate);
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

    resolve_dentry(ctx, DR_KPROBE_OR_FENTRY);

    // if the tail call fails, we need to pop the syscall cache entry
    pop_syscall_with(security_inode_predicate);

    return 0;
}

TAIL_CALL_TARGET("dr_setattr_callback")
int tail_call_target_dr_setattr_callback(ctx_t *ctx) {
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
