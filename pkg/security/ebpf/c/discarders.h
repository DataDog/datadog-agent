#ifndef _DISCARDERS_H
#define _DISCARDERS_H

#define REVISION_ARRAY_SIZE 4096

struct bpf_map_def SEC("maps/discarder_revisions") discarder_revisions = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = REVISION_ARRAY_SIZE,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) get_discarder_revision(u32 mount_id) {
    u32 i = mount_id % REVISION_ARRAY_SIZE;
    u32 *revision = bpf_map_lookup_elem(&discarder_revisions, &i);

    return revision ? *revision : 0;
}

int __attribute__((always_inline)) bump_discarder_revision(u32 mount_id) {
    u32 i = mount_id % REVISION_ARRAY_SIZE;
    u32 *revision = bpf_map_lookup_elem(&discarder_revisions, &i);
    if (!revision) {
        return 0;
    }

    // bump only already > 0 meaning that the user space decided that for this mount_id
    // all the discarders will be invalidated
    if (*revision > 0) {
        if (*revision + 1 == 0) {
            __sync_fetch_and_add(revision, 2); // handle overflow
        } else {
            __sync_fetch_and_add(revision, 1);
        }
    }

    return *revision;
}

struct inode_discarder_t {
    struct path_key_t path_key;
    u32 revision;
    u32 padding;
};

struct inode_filter_t {
    u64 parent_mask;
    u64 leaf_mask;
};

struct bpf_map_def SEC("maps/inode_discarders") inode_discarders = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct inode_discarder_t),
    .value_size = sizeof(struct inode_filter_t),
    .max_entries = 512,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) discarded_by_inode(u64 event_type, u32 mount_id, u64 inode, u64 depth) {
    struct inode_discarder_t key = {
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        },
        .revision = get_discarder_revision(mount_id),
    };

    struct inode_filter_t *filter = bpf_map_lookup_elem(&inode_discarders, &key);
    if (!filter) {
        return 0;
    }

    // this a filter for leaf only
    if (depth == 0 && mask_has_event(filter->leaf_mask, event_type)) {
        return 1;
    }

    if (depth > 0 && mask_has_event(filter->parent_mask, event_type)) {
        return 1;
    }

    return 0;
}

void __attribute__((always_inline)) remove_inode_discarder(u32 mount_id, u64 inode) {
    struct inode_discarder_t key = {
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        },
        .revision = get_discarder_revision(mount_id),
    };

    bpf_map_delete_elem(&inode_discarders, &key);
}

static __always_inline u32 get_system_probe_pid() {
    u64 val = 0;
    LOAD_CONSTANT("system_probe_pid", val);
    return val;
}

struct pid_discarder_t {
    u32 tgid;
};

struct pid_discarder_parameters_t {
    u64 event_mask;
    u64 timestamps[EVENT_MAX_ROUNDED_UP];
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
    u32 system_probe_pid = get_system_probe_pid();
    if (system_probe_pid && system_probe_pid == tgid) {
        return 1;
    }

    struct pid_discarder_t key = {
        .tgid = tgid,
    };

    struct pid_discarder_parameters_t *params = bpf_map_lookup_elem(&pid_discarders, &key);

    if (params == NULL || (event_type > 0 && params->timestamps[(event_type-1)&(EVENT_MAX_ROUNDED_UP-1)] != 0 && params->timestamps[(event_type-1)&(EVENT_MAX_ROUNDED_UP-1)] <= bpf_ktime_get_ns())) {
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

        struct proc_cache_t *entry = get_proc_cache(tgid);
        if (entry && discarded_by_inode(event_type, entry->executable.mount_id, entry->executable.inode, 0)) {
            return 1;
        }
    }

    return 0;
}

#endif
