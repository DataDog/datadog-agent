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
    } else if (approver_type == AUID_APPROVER_TYPE) {
        __sync_fetch_and_add(&stats->event_approved_by_auid, 1);
    }
}

void get_dentry_name(struct dentry *dentry, void *buffer, size_t n);

enum SYSCALL_STATE __attribute__((always_inline)) approve_by_auid(struct syscall_cache_t *syscall, u64 event_type) {
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    struct pid_cache_t *pid_entry = (struct pid_cache_t *)bpf_map_lookup_elem(&pid_cache, &pid);
    if (!pid_entry || !pid_entry->credentials.is_auid_set) {
        return DISCARDED;
    }

    u32 auid = pid_entry->credentials.auid;

    struct event_mask_filter_t *mask_filter = bpf_map_lookup_elem(&auid_approvers, &auid);
    if (mask_filter && mask_filter->event_mask & (1 << (event_type - 1))) {
        monitor_event_approved(syscall->type, AUID_APPROVER_TYPE);
        return ACCEPTED;
    }

    struct u32_range_filter_t *range_filter = bpf_map_lookup_elem(&auid_range_approvers, &event_type);
    if (range_filter && auid >= range_filter->min && auid <= range_filter->max) {
        monitor_event_approved(syscall->type, AUID_APPROVER_TYPE);
        return ACCEPTED;
    }

    return DISCARDED;
}

enum SYSCALL_STATE __attribute__((always_inline)) approve_by_basename(struct dentry *dentry, u64 event_type) {
    struct basename_t basename = {};
    get_dentry_name(dentry, &basename, sizeof(basename));

    struct event_mask_filter_t *filter = bpf_map_lookup_elem(&basename_approvers, &basename);
    if (filter && filter->event_mask & (1 << (event_type - 1))) {
        monitor_event_approved(event_type, BASENAME_APPROVER_TYPE);
        return APPROVED;
    }
    return DISCARDED;
}

enum SYSCALL_STATE __attribute__((always_inline)) chmod_approvers(struct syscall_cache_t *syscall) {
    return approve_by_basename(syscall->setattr.dentry, EVENT_CHMOD);
}

enum SYSCALL_STATE __attribute__((always_inline)) chown_approvers(struct syscall_cache_t *syscall) {
    return approve_by_basename(syscall->setattr.dentry, EVENT_CHOWN);
}

int __attribute__((always_inline)) lookup_u32_flags(void *map, u32 *flags) {
    u32 key = 0;
    struct u32_flags_filter_t *filter = bpf_map_lookup_elem(map, &key);
    if (filter == NULL || !filter->is_set) {
        return 0;
    }
    *flags = filter->flags;

    return 1;
}

int __attribute__((always_inline)) approve_mmap_by_flags(struct syscall_cache_t *syscall) {
    u32 flags = 0;

    int exists = lookup_u32_flags(&mmap_flags_approvers, &flags);
    if (!exists) {
        return DISCARDED;
    }

    if ((syscall->mmap.flags & flags) > 0) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
        return APPROVED;
    }
    return DISCARDED;
}

enum SYSCALL_STATE __attribute__((always_inline)) approve_mmap_by_protection(struct syscall_cache_t *syscall) {
    u32 flags = 0;

    int exists = lookup_u32_flags(&mmap_protection_approvers, &flags);
    if (!exists) {
        return DISCARDED;
    }

    if ((flags == 0 && syscall->mmap.protection == 0) || (syscall->mmap.protection & flags) > 0) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
        return APPROVED;
    }
    return DISCARDED;
}

enum SYSCALL_STATE __attribute__((always_inline)) mmap_approvers(struct syscall_cache_t *syscall) {
    enum SYSCALL_STATE state = DISCARDED;

    if (syscall->mmap.dentry != NULL) {
        state = approve_by_basename(syscall->mmap.dentry, EVENT_MMAP);
    }

    if (state == DISCARDED) {
        state = approve_mmap_by_protection(syscall);
    }
    if (state == DISCARDED) {
        state = approve_mmap_by_flags(syscall);
    }

    return state;
}

enum SYSCALL_STATE __attribute__((always_inline)) link_approvers(struct syscall_cache_t *syscall) {
    enum SYSCALL_STATE state = approve_by_basename(syscall->link.src_dentry, EVENT_LINK);
    if (state == DISCARDED) {
        state = approve_by_basename(syscall->link.target_dentry, EVENT_LINK);
    }

    return state;
}

enum SYSCALL_STATE __attribute__((always_inline)) mkdir_approvers(struct syscall_cache_t *syscall) {
    return approve_by_basename(syscall->mkdir.dentry, EVENT_MKDIR);
}

enum SYSCALL_STATE __attribute__((always_inline)) chdir_approvers(struct syscall_cache_t *syscall) {
    return approve_by_basename(syscall->chdir.dentry, EVENT_CHDIR);
}

enum SYSCALL_STATE __attribute__((always_inline)) approve_mprotect_by_vm_protection(struct syscall_cache_t *syscall) {
    u32 flags = 0;

    int exists = lookup_u32_flags(&mprotect_vm_protection_approvers, &flags);
    if (!exists) {
        return DISCARDED;
    }

    if ((syscall->mprotect.vm_protection & flags) > 0) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
        return APPROVED;
    }
    return DISCARDED;
}

enum SYSCALL_STATE __attribute__((always_inline)) approve_mprotect_by_req_protection(struct syscall_cache_t *syscall) {
    u32 flags = 0;

    int exists = lookup_u32_flags(&mprotect_req_protection_approvers, &flags);
    if (!exists) {
        return DISCARDED;
    }

    if ((syscall->mprotect.req_protection & flags) > 0) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
        return APPROVED;
    }
    return DISCARDED;
}

enum SYSCALL_STATE __attribute__((always_inline)) mprotect_approvers(struct syscall_cache_t *syscall) {
    enum SYSCALL_STATE state = approve_mprotect_by_vm_protection(syscall);
    if (state == DISCARDED) {
        state = approve_mprotect_by_req_protection(syscall);
    }

    return state;
}

enum SYSCALL_STATE __attribute__((always_inline)) approve_open_by_flags(struct syscall_cache_t *syscall) {
    u32 flags = 0;

    int exists = lookup_u32_flags(&open_flags_approvers, &flags);
    if (!exists) {
        return DISCARDED;
    }

    if ((flags == 0 && syscall->open.flags == 0) || ((syscall->open.flags & flags) > 0)) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);

#if defined(DEBUG_APPROVERS)
        bpf_printk("open flags %d approved", syscall->open.flags);
#endif

        return APPROVED;
    }
    return DISCARDED;
}

enum SYSCALL_STATE __attribute__((always_inline)) open_approvers(struct syscall_cache_t *syscall) {
    enum SYSCALL_STATE state = approve_by_basename(syscall->open.dentry, EVENT_OPEN);
    if (state == DISCARDED) {
        state = approve_open_by_flags(syscall);
    }
    if (state == DISCARDED) {
        state = approve_by_auid(syscall, EVENT_OPEN);
    }

    return state;
}

enum SYSCALL_STATE __attribute__((always_inline)) rename_approvers(struct syscall_cache_t *syscall) {
    enum SYSCALL_STATE state = approve_by_basename(syscall->rename.src_dentry, EVENT_RENAME);
    if (state == DISCARDED) {
        state = approve_by_basename(syscall->rename.target_dentry, EVENT_RENAME);
    }

    return state;
}

enum SYSCALL_STATE __attribute__((always_inline)) rmdir_approvers(struct syscall_cache_t *syscall) {
    return approve_by_basename(syscall->rmdir.dentry, EVENT_RMDIR);
}

enum SYSCALL_STATE __attribute__((always_inline)) approve_splice_by_entry_flags(struct syscall_cache_t *syscall) {
    u32 flags = 0;

    int exists = lookup_u32_flags(&splice_entry_flags_approvers, &flags);
    if (!exists) {
        return DISCARDED;
    }

    if ((syscall->splice.pipe_entry_flag & flags) > 0) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
        return APPROVED;
    }
    return DISCARDED;
}

enum SYSCALL_STATE __attribute__((always_inline)) approve_splice_by_exit_flags(struct syscall_cache_t *syscall) {
    u32 flags = 0;

    int exists = lookup_u32_flags(&splice_exit_flags_approvers, &flags);
    if (!exists) {
        return DISCARDED;
    }

    if ((syscall->splice.pipe_exit_flag & flags) > 0) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
        return APPROVED;
    }
    return DISCARDED;
}

enum SYSCALL_STATE __attribute__((always_inline)) splice_approvers(struct syscall_cache_t *syscall) {
    enum SYSCALL_STATE state = DISCARDED;

    if (syscall->splice.dentry != NULL) {
        state = approve_by_basename(syscall->splice.dentry, EVENT_SPLICE);
    }

    if (state == DISCARDED) {
        state = approve_splice_by_exit_flags(syscall);
    }
    if (state == DISCARDED) {
        state = approve_splice_by_entry_flags(syscall);
    }

    return state;
}

enum SYSCALL_STATE __attribute__((always_inline)) unlink_approvers(struct syscall_cache_t *syscall) {
    return approve_by_basename(syscall->unlink.dentry, EVENT_UNLINK);
}

enum SYSCALL_STATE __attribute__((always_inline)) utime_approvers(struct syscall_cache_t *syscall) {
    return approve_by_basename(syscall->setattr.dentry, EVENT_UTIME);
}

enum SYSCALL_STATE __attribute__((always_inline)) bpf_approvers(struct syscall_cache_t *syscall) {
    u32 key = 0;
    struct u64_flags_filter_t *filter = bpf_map_lookup_elem(&bpf_cmd_approvers, &key);
    if (filter == NULL || !filter->is_set) {
        return DISCARDED;
    }

    if (((1 << syscall->bpf.cmd) & filter->flags) > 0) {
        monitor_event_approved(syscall->type, FLAG_APPROVER_TYPE);
        return APPROVED;
    }

    return DISCARDED;
}

enum SYSCALL_STATE __attribute__((always_inline)) approve_syscall(struct syscall_cache_t *syscall, enum SYSCALL_STATE (*check_approvers)(struct syscall_cache_t *syscall)) {
    if (syscall->policy.mode == NO_FILTER) {
        return syscall->state = ACCEPTED;
    }

    if (syscall->policy.mode == ACCEPT) {
        return syscall->state = APPROVED;
    }

    if (syscall->policy.mode == DENY) {
        syscall->state = check_approvers(syscall);
    }

    u32 tgid = bpf_get_current_pid_tgid() >> 32;
    u64 *cookie = bpf_map_lookup_elem(&traced_pids, &tgid);
    if (cookie != NULL) {
        u64 now = bpf_ktime_get_ns();
        struct activity_dump_config *config = lookup_or_delete_traced_pid(tgid, now, cookie);
        if (config != NULL) {
            // is this event type traced ?
            if (mask_has_event(config->event_mask, syscall->type) && activity_dump_rate_limiter_allow(config, *cookie, now, 0)) {
                if (syscall->state == DISCARDED) {
                    syscall->resolver.flags |= SAVED_BY_ACTIVITY_DUMP;
                }

                // force to be accepted as this event will be part of a dump
                syscall->state = ACCEPTED;
            }
        }
    }

    return syscall->state;
}

#endif
