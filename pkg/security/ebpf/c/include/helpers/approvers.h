#ifndef _APPROVERS_H
#define _APPROVERS_H

#include "constants/enums.h"
#include "maps.h"

void get_dentry_name(struct dentry *dentry, void *buffer, size_t n);

int __attribute__((always_inline)) approve_by_basename(struct dentry *dentry, u64 event_type) {
    struct basename_t basename = {};
    get_dentry_name(dentry, &basename, sizeof(basename));

    struct basename_filter_t *filter = bpf_map_lookup_elem(&basename_approvers, &basename);
    if (filter && filter->event_mask & (1 << (event_type-1))) {
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) basename_approver(struct syscall_cache_t *syscall, struct dentry *dentry, u64 event_type) {
    if ((syscall->policy.flags & BASENAME) > 0) {
        return approve_by_basename(dentry, event_type);
    }
    return 0;
}

int __attribute__((always_inline)) chmod_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->setattr.dentry, EVENT_CHMOD);
}

int __attribute__((always_inline)) chown_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->setattr.dentry, EVENT_CHOWN);
}

int __attribute__((always_inline)) approve_mmap_by_flags(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&mmap_flags_approvers, &key);
    if (flags != NULL && (syscall->mmap.flags & *flags) > 0) {
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) approve_mmap_by_protection(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&mmap_protection_approvers, &key);
    if (flags != NULL && (syscall->mmap.protection & *flags) > 0) {
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) mmap_approvers(struct syscall_cache_t *syscall) {
    int pass_to_userspace = 0;

    if ((syscall->policy.flags & BASENAME) > 0 && syscall->mmap.dentry != NULL) {
        pass_to_userspace = approve_by_basename(syscall->mmap.dentry, EVENT_MMAP);
    }

    if (!pass_to_userspace && (syscall->policy.flags & FLAGS) > 0) {
        pass_to_userspace = approve_mmap_by_protection(syscall);
        if (!pass_to_userspace) {
            pass_to_userspace = approve_mmap_by_flags(syscall);
        }
    }

    return pass_to_userspace;
}

int __attribute__((always_inline)) link_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->link.src_dentry, EVENT_LINK) ||
           basename_approver(syscall, syscall->link.target_dentry, EVENT_LINK);
}

int __attribute__((always_inline)) mkdir_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->mkdir.dentry, EVENT_MKDIR);
}

int __attribute__((always_inline)) approve_mprotect_by_vm_protection(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&mprotect_vm_protection_approvers, &key);
    if (flags != NULL && (syscall->mprotect.vm_protection & *flags) > 0) {
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) approve_mprotect_by_req_protection(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&mprotect_req_protection_approvers, &key);
    if (flags != NULL && (syscall->mprotect.req_protection & *flags) > 0) {
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) mprotect_approvers(struct syscall_cache_t *syscall) {
    int pass_to_userspace = 0;

    if ((syscall->policy.flags & FLAGS) > 0) {
        int vm_protection_approved = approve_mprotect_by_vm_protection(syscall);
        int req_protection_approved = approve_mprotect_by_req_protection(syscall);
        pass_to_userspace = vm_protection_approved && req_protection_approved;
    }

    return pass_to_userspace;
}

int __attribute__((always_inline)) approve_by_flags(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&open_flags_approvers, &key);
    if (flags != NULL && (syscall->open.flags & *flags) > 0) {
#ifdef DEBUG
        bpf_printk("open flags %d approved\n", syscall->open.flags);
#endif
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) open_approvers(struct syscall_cache_t *syscall) {
    int pass_to_userspace = 0;

    if ((syscall->policy.flags & BASENAME) > 0) {
        pass_to_userspace = approve_by_basename(syscall->open.dentry, EVENT_OPEN);
    }

    if (!pass_to_userspace && (syscall->policy.flags & FLAGS) > 0) {
        pass_to_userspace = approve_by_flags(syscall);
    }

    return pass_to_userspace;
}

int __attribute__((always_inline)) rename_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->rename.src_dentry, EVENT_RENAME) ||
           basename_approver(syscall, syscall->rename.target_dentry, EVENT_RENAME);
}

int __attribute__((always_inline)) rmdir_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->rmdir.dentry, EVENT_RMDIR);
}

int __attribute__((always_inline)) approve_splice_by_entry_flags(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&splice_entry_flags_approvers, &key);
    if (flags != NULL && (syscall->splice.pipe_entry_flag & *flags) > 0) {
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) approve_splice_by_exit_flags(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&splice_exit_flags_approvers, &key);
    if (flags != NULL && (syscall->splice.pipe_exit_flag & *flags) > 0) {
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) splice_approvers(struct syscall_cache_t *syscall) {
    int pass_to_userspace = 0;

    if ((syscall->policy.flags & BASENAME) > 0 && syscall->splice.dentry != NULL) {
        pass_to_userspace = approve_by_basename(syscall->splice.dentry, EVENT_SPLICE);
    }

    if (!pass_to_userspace && (syscall->policy.flags & FLAGS) > 0) {
        pass_to_userspace = approve_splice_by_exit_flags(syscall);
        if (!pass_to_userspace) {
            pass_to_userspace = approve_splice_by_entry_flags(syscall);
        }
    }

    return pass_to_userspace;
}

int __attribute__((always_inline)) unlink_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->unlink.dentry, EVENT_UNLINK);
}

int __attribute__((always_inline)) utime_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->setattr.dentry, EVENT_UTIME);
}

#endif
