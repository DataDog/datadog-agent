#ifndef _FILTERS_H
#define _FILTERS_H

#include "process.h"

enum policy_mode
{
    ACCEPT = 1,
    DENY = 2,
    NO_FILTER = 3,
};

enum policy_flags
{
    BASENAME = 1,
    FLAGS = 2,
    MODE = 4,
    PARENT_NAME = 8,
};

struct policy_t {
    char mode;
    char flags;
};

struct filter_t {
    u64 event_mask;
};

struct bpf_map_def SEC("maps/filter_policy") filter_policy = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct policy_t),
    .max_entries = EVENT_MAX,
    .pinning = 0,
    .namespace = "",
};

struct inode_discarder_t {
    struct path_key_t path_key;
};

struct bpf_map_def SEC("maps/inode_discarders") inode_discarders = { \
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct inode_discarder_t),
    .value_size = sizeof(struct filter_t),
    .max_entries = 512,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) discarded_by_inode(u64 event_type, u32 mount_id, u64 inode) {
    struct inode_discarder_t key = {
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        }
    };

    struct filter_t *filter = bpf_map_lookup_elem(&inode_discarders, &key);

    if (filter && mask_has_event(filter->event_mask, event_type)) {
#ifdef DEBUG
        bpf_printk("file with inode %d discarded\n", inode);
#endif
        return 1;
    }
    return 0;
}

void __attribute__((always_inline)) remove_inode_discarder(u32 mount_id, u64 inode) {
    struct inode_discarder_t key = {
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        }
    };

    bpf_map_delete_elem(&inode_discarders, &key);
}

struct pid_discarder_t {
    u32 tgid;
};

struct pid_discarder_parameters_t {
    u64 event_mask;
    u64 timestamps[EVENT_MAX];
};

struct bpf_map_def SEC("maps/pid_discarders") pid_discarders = { \
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct pid_discarder_parameters_t),
    .max_entries = 512,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) discarded_by_pid(u64 event_type, u32 tgid) {
    struct pid_discarder_t key = {
        .tgid = tgid,
    };

    struct pid_discarder_parameters_t *params = bpf_map_lookup_elem(&pid_discarders, &key);

    if (params == NULL || (event_type > 0 && params->timestamps[(event_type-1)&(EVENT_MAX-1)] != 0 && params->timestamps[(event_type-1)&(EVENT_MAX-1)] <= bpf_ktime_get_ns())) {
        return 0;
    }

#ifdef DEBUG
        bpf_printk("process with pid %d discarded\n", tgid);
#endif
    return mask_has_event(params->event_mask, event_type);
}

// cache_syscall checks the event policy in order to see if the syscall struct can be cached
int __attribute__((always_inline)) discarded_by_process(const char mode, u64 event_type) {
    if (mode != NO_FILTER) {
        u64 pid_tgid = bpf_get_current_pid_tgid();
        u32 tgid = pid_tgid >> 32;

        // try with pid first
        if (discarded_by_pid(event_type, tgid))
            return 1;

        struct proc_cache_t *entry = get_pid_cache(tgid);
        if (entry && discarded_by_inode(event_type, entry->executable.mount_id, entry->executable.inode)) {
            return 1;
        }
    }

    return 0;
}

#endif
