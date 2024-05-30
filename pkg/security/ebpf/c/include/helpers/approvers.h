#ifndef _APPROVERS_H
#define _APPROVERS_H

#include "constants/enums.h"
#include "maps.h"

void __attribute__((always_inline)) monitor_event_approved(u64 event_type, u32 approver_type) {
    struct bpf_map_def *approver_stats = select_buffer(&fb_approver_stats, &bb_approver_stats, APPROVER_MONITOR_KEY);
    if (approver_stats == NULL) {
        return;
    }

    u32 key = event_type;
    struct approver_stats_t *stats = bpf_map_lookup_elem(approver_stats, &key);
    if (stats == NULL) {
        return;
    }

    if (approver_type == BASENAME_APPROVER_TYPE) {
        __sync_fetch_and_add(&stats->event_approved_by_basename, 1);
    } else if (approver_type == FLAG_APPROVER_TYPE) {
        __sync_fetch_and_add(&stats->event_approved_by_flag, 1);
    }
}

void get_dentry_name(struct dentry *dentry, void *buffer, size_t n);

int __attribute__((always_inline)) approve_by_basename(struct dentry *dentry, u64 event_type) {
    struct basename_t basename = {};
    get_dentry_name(dentry, &basename, sizeof(basename));

    struct basename_filter_t *filter = bpf_map_lookup_elem(&basename_approvers, &basename);
    if (filter && filter->event_mask & (1 << (event_type-1))) {
        monitor_event_approved(event_type, BASENAME_APPROVER_TYPE);
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
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) approve_mmap_by_protection(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags_ptr = bpf_map_lookup_elem(&mmap_protection_approvers, &key);
    if (flags_ptr == NULL) {
        return 0;
    }
    u32 flags = *flags_ptr;
    if ((flags == 0 && syscall->mmap.protection == 0) || (syscall->mmap.protection & flags) > 0) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
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

int __attribute__((always_inline)) chdir_approvers(struct syscall_cache_t *syscall) {
    return basename_approver(syscall, syscall->chdir.dentry, EVENT_CHDIR);
}

int __attribute__((always_inline)) approve_mprotect_by_vm_protection(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&mprotect_vm_protection_approvers, &key);
    if (flags != NULL && (syscall->mprotect.vm_protection & *flags) > 0) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) approve_mprotect_by_req_protection(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&mprotect_req_protection_approvers, &key);
    if (flags != NULL && (syscall->mprotect.req_protection & *flags) > 0) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) mprotect_approvers(struct syscall_cache_t *syscall) {
    int pass_to_userspace = 0;

    if ((syscall->policy.flags & FLAGS) > 0) {
        pass_to_userspace = approve_mprotect_by_vm_protection(syscall);
        if (!pass_to_userspace) {
            pass_to_userspace = approve_mprotect_by_req_protection(syscall);
        }
    }

    return pass_to_userspace;
}

int __attribute__((always_inline)) approve_by_flags(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags_ptr = bpf_map_lookup_elem(&open_flags_approvers, &key);
    if (flags_ptr == NULL) {
        return 0;
    }

    u32 flags = *flags_ptr;
    if ((flags == 0 && syscall->open.flags == 0) || ((syscall->open.flags & flags) > 0)) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
#ifdef DEBUG
        bpf_printk("open flags %d approved", syscall->open.flags);
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
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
        return 1;
    }
    return 0;
}

int __attribute__((always_inline)) approve_splice_by_exit_flags(struct syscall_cache_t *syscall) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&splice_exit_flags_approvers, &key);
    if (flags != NULL && (syscall->splice.pipe_exit_flag & *flags) > 0) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
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

int __attribute__((always_inline)) bpf_approvers(struct syscall_cache_t *syscall) {
    int pass_to_userspace = 0;

    if ((syscall->policy.flags & FLAGS) > 0) {
        u32 key = 0;
        u64 *cmd_bitmask = bpf_map_lookup_elem(&bpf_cmd_approvers, &key);
        if (cmd_bitmask != NULL && ((1 << syscall->bpf.cmd) & *cmd_bitmask) > 0) {
            monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
            pass_to_userspace = 1;
        }
    }

    return pass_to_userspace;
}

#endif
