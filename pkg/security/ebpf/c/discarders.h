#ifndef _DISCARDERS_H
#define _DISCARDERS_H

#define REVISION_ARRAY_SIZE 4096

#define INODE_DISCARDER_TYPE 0
#define PID_DISCARDER_TYPE   1

struct discarder_stats_t {
    u64 discarders_added;
    u64 event_discarded;
};

struct bpf_map_def SEC("maps/discarder_stats_fb") discarder_stats_fb = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct discarder_stats_t),
    .max_entries = EVENT_LAST_DISCARDER,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/discarder_stats_bb") discarder_stats_bb = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct discarder_stats_t),
    .max_entries = EVENT_LAST_DISCARDER,
    .pinning = 0,
    .namespace = "",
};

struct bpf_map_def SEC("maps/inode_discarder_revisions") inode_discarder_revisions = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = REVISION_ARRAY_SIZE,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) monitor_discarder_added(u64 event_type) {
    struct bpf_map_def *discarder_stats = select_buffer(&discarder_stats_fb, &discarder_stats_bb, DISCARDER_MONITOR_KEY);
    if (discarder_stats == NULL) {
        return 0;
    }

    u32 key = event_type;
    struct discarder_stats_t *stats = bpf_map_lookup_elem(discarder_stats, &key);
    if (stats == NULL) {
        return 0;
    }

    __sync_fetch_and_add(&stats->discarders_added, 1);

    return 0;
}

int __attribute__((always_inline)) monitor_discarded(u64 event_type) {
    struct bpf_map_def *discarder_stats = select_buffer(&discarder_stats_fb, &discarder_stats_bb, DISCARDER_MONITOR_KEY);
    if (discarder_stats == NULL) {
        return 0;
    }

    u32 key = event_type;
    struct discarder_stats_t *stats = bpf_map_lookup_elem(discarder_stats, &key);
    if (stats == NULL) {
        return 0;
    }

    __sync_fetch_and_add(&stats->event_discarded, 1);

    return 0;
}

int __attribute__((always_inline)) get_inode_discarder_revision(u32 mount_id) {
    u32 i = mount_id % REVISION_ARRAY_SIZE;
    u32 *revision = bpf_map_lookup_elem(&inode_discarder_revisions, &i);

    return revision ? *revision : 0;
}

int __attribute__((always_inline)) bump_inode_discarder_revision(u32 mount_id) {
    u32 i = mount_id % REVISION_ARRAY_SIZE;
    u32 *revision = bpf_map_lookup_elem(&inode_discarder_revisions, &i);
    if (!revision) {
        return 0;
    }

    // bump only already > 0 meaning that the user space decided that for this mount_id
    // all the discarders will be invalidated
    __sync_fetch_and_add(revision, 1);

    return *revision;
}

struct discarder_params_t {
    u64 event_mask;
    u64 timestamps[EVENT_LAST_DISCARDER-EVENT_FIRST_DISCARDER];
    u64 expire_at;
    u32 is_retained;
};

u64* __attribute__((always_inline)) get_discarder_timestamp(struct discarder_params_t *params, u64 event_type) {
    switch (event_type) {
        case EVENT_OPEN:
            return &params->timestamps[EVENT_OPEN-EVENT_FIRST_DISCARDER];
        case EVENT_MKDIR:
            return &params->timestamps[EVENT_MKDIR-EVENT_FIRST_DISCARDER];
        case EVENT_LINK:
            return &params->timestamps[EVENT_LINK-EVENT_FIRST_DISCARDER];
        case EVENT_RENAME:
            return &params->timestamps[EVENT_RENAME-EVENT_FIRST_DISCARDER];
        case EVENT_UNLINK:
            return &params->timestamps[EVENT_UNLINK-EVENT_FIRST_DISCARDER];
        case EVENT_RMDIR:
            return &params->timestamps[EVENT_RMDIR-EVENT_FIRST_DISCARDER];
        case EVENT_CHMOD:
            return &params->timestamps[EVENT_CHMOD-EVENT_FIRST_DISCARDER];
        case EVENT_CHOWN:
            return &params->timestamps[EVENT_CHOWN-EVENT_FIRST_DISCARDER];
        case EVENT_UTIME:
            return &params->timestamps[EVENT_UTIME-EVENT_FIRST_DISCARDER];
        case EVENT_SETXATTR:
            return &params->timestamps[EVENT_SETXATTR-EVENT_FIRST_DISCARDER];
        case EVENT_REMOVEXATTR:
            return &params->timestamps[EVENT_REMOVEXATTR-EVENT_FIRST_DISCARDER];
        default:
            return NULL;
    }
}

// This function is doing the same thing as the one before, but can only work if `params` is a pointer to a map value
// and not a pointer to the stack since kernels < 4.15 does not allow this. On the other hand it is faster and needs less
// instructions.
u64* __attribute__((always_inline)) get_discarder_timestamp_from_map(struct discarder_params_t *params, u64 event_type) {
    if (EVENT_FIRST_DISCARDER <= event_type && event_type < EVENT_LAST_DISCARDER) {
        return &params->timestamps[event_type-EVENT_FIRST_DISCARDER];
    }
    return NULL;
}

void * __attribute__((always_inline)) is_discarded(struct bpf_map_def *discarder_map, void *key, u64 event_type, u64 now) {
    void *entry = bpf_map_lookup_elem(discarder_map, key);
    if (entry == NULL) {
        return NULL;
    }

    struct discarder_params_t *params = (struct discarder_params_t *)entry;

    // this discarder has been marked as on hold by event such as unlink, rename, etc.
    // keep them for a while in the map to avoid userspace to reinsert it with a pending userspace event
    if (params->is_retained) {
        if (params->expire_at < now) {
            // important : never modify the discarder maps during the flush as may corrupt the iteration
            if (!is_flushing_discarders()) {
                bpf_map_delete_elem(discarder_map, key);
            }
        }
        return NULL;
    }

    u64* pid_tm = get_discarder_timestamp_from_map(params, event_type);
    if (pid_tm != NULL && *pid_tm && *pid_tm <= now) {
        return NULL;
    }

    if (mask_has_event(params->event_mask, event_type)) {
        return entry;
    }

    return NULL;
}

struct inode_discarder_params_t {
    struct discarder_params_t params;
    u32 revision;
};

struct bpf_map_def SEC("maps/inode_discarders") inode_discarders = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(struct inode_discarder_t),
    .value_size = sizeof(struct inode_discarder_params_t),
    .max_entries = 4096,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) discard_inode(u64 event_type, u32 mount_id, u64 inode, u64 timeout, u32 is_leaf) {
    if (!mount_id || !inode) {
        return 0;
    }

    struct inode_discarder_t key = {
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        },
        .is_leaf = is_leaf,
    };

    u64 *discarder_timestamp;
    u64 timestamp = timeout ? bpf_ktime_get_ns() + timeout : 0;
    int revision = get_inode_discarder_revision(mount_id);

    struct inode_discarder_params_t *inode_params = bpf_map_lookup_elem(&inode_discarders, &key);
    if (inode_params) {
        u64 tm = bpf_ktime_get_ns();
        if (inode_params->params.is_retained && inode_params->params.expire_at < tm) {
            inode_params->params.is_retained = 0;

            // the revision change, all the discarders are invalidated,
            // we need to add only the current event type and to use the current revision
            if (inode_params->revision != revision) {
                inode_params->params.event_mask = 0;
                inode_params->revision = revision;
            }
            add_event_to_mask(&inode_params->params.event_mask, event_type);

            if ((discarder_timestamp = get_discarder_timestamp(&inode_params->params, event_type)) != NULL) {
                *discarder_timestamp = timestamp;
            }
        }
    } else {
        struct inode_discarder_params_t new_inode_params = {
            .revision = revision,
        };
        add_event_to_mask(&new_inode_params.params.event_mask, event_type);

        if ((discarder_timestamp = get_discarder_timestamp(&new_inode_params.params, event_type)) != NULL) {
            *discarder_timestamp = timestamp;
        }
        bpf_map_update_elem(&inode_discarders, &key, &new_inode_params, BPF_NOEXIST);
    }

    monitor_discarder_added(event_type);

    return 0;
}

typedef enum discard_check_state {
    NOT_DISCARDED,
    DISCARDED,
    SAVED_BY_AD,
} discard_check_state;

discard_check_state __attribute__((always_inline)) is_discarded_by_inode(struct is_discarded_by_inode_t *params) {
    // start with the "normal" discarder check
    struct inode_discarder_t key = params->discarder;
    struct inode_discarder_params_t *inode_params = (struct inode_discarder_params_t *) is_discarded(&inode_discarders, &key, params->discarder_type, params->now);
    if (!inode_params) {
        return NOT_DISCARDED;
    }

    bool is_discarded = inode_params->revision == get_inode_discarder_revision(params->discarder.path_key.mount_id);
    if (!is_discarded) {
        return NOT_DISCARDED;
    }

    // should we ignore the discarder check because of an activity dump ?
    if (params->ad_state == ACTIVITY_DUMP_RUNNING) {
        // do not discard this event
        return SAVED_BY_AD;
    }
    return DISCARDED;
}

void __attribute__((always_inline)) expire_inode_discarders(u32 mount_id, u64 inode) {
    if (!mount_id || !inode) {
        return;
    }

    u64 retention;
    LOAD_CONSTANT("discarder_retention", retention);

    u64 expire_at = bpf_ktime_get_ns() + retention;

    struct inode_discarder_t key = {
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        }
    };

    struct inode_discarder_params_t new_inode_params = {
        .params = {
            .is_retained = 1,
            .expire_at = expire_at,
        },
        .revision = get_inode_discarder_revision(mount_id),
    };

    #pragma unroll
    for (int i = 0; i != 2; i++) {
        key.is_leaf = i;

        struct inode_discarder_params_t *inode_params = bpf_map_lookup_elem(&inode_discarders, &key);
        if (inode_params) {
            inode_params->params.is_retained = 1;
            inode_params->params.expire_at = expire_at;
        } else {
            // add a retention anyway
            bpf_map_update_elem(&inode_discarders, &key, &new_inode_params, BPF_NOEXIST);
        }
    }
}

static __always_inline u32 is_runtime_discarded() {
    u64 discarded = 0;
    LOAD_CONSTANT("runtime_discarded", discarded);
    return discarded;
}

struct pid_discarder_params_t {
    struct discarder_params_t params;
};

struct pid_discarder_t {
    u32 tgid;
};

struct bpf_map_def SEC("maps/pid_discarders") pid_discarders = { \
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct pid_discarder_params_t),
    .max_entries = 512,
    .pinning = 0,
    .namespace = "",
};

int __attribute__((always_inline)) discard_pid(u64 event_type, u32 tgid, u64 timeout) {
    struct pid_discarder_t key = {
        .tgid = tgid,
    };

    u64 *discarder_timestamp;
    u64 timestamp = timeout ? bpf_ktime_get_ns() + timeout : 0;

    struct pid_discarder_params_t *pid_params = bpf_map_lookup_elem(&pid_discarders, &key);
    if (pid_params) {
        add_event_to_mask(&pid_params->params.event_mask, event_type);

        if ((discarder_timestamp = get_discarder_timestamp_from_map(&pid_params->params, event_type)) != NULL) {
            *discarder_timestamp = timestamp;
        }

        u64 tm = bpf_ktime_get_ns();
        if (pid_params->params.is_retained && pid_params->params.expire_at < tm) {
            pid_params->params.is_retained = 0;
        }
    } else {
        struct pid_discarder_params_t new_pid_params = {};
        add_event_to_mask(&new_pid_params.params.event_mask, event_type);

        if ((discarder_timestamp = get_discarder_timestamp(&new_pid_params.params, event_type)) != NULL) {
            *discarder_timestamp = timestamp;
        }
        bpf_map_update_elem(&pid_discarders, &key, &new_pid_params, BPF_NOEXIST);
    }

    monitor_discarder_added(EVENT_ANY);

    return 0;
}

int __attribute__((always_inline)) is_discarded_by_pid(u64 event_type, u32 tgid) {
    struct pid_discarder_t key = {
        .tgid = tgid,
    };

    return is_discarded(&pid_discarders, &key, event_type, bpf_ktime_get_ns()) != NULL;
}

int __attribute__((always_inline)) is_discarded_by_process(const char mode, u64 event_type) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    u64 runtime_pid;
    LOAD_CONSTANT("runtime_pid", runtime_pid);

    if (is_runtime_discarded() && runtime_pid == tgid) {
        return 1;
    }

    if (mode != NO_FILTER && is_discarded_by_pid(event_type, tgid)) {
        return 1;
    }

    return 0;
}

void __attribute__((always_inline)) expire_pid_discarder(u32 tgid) {
    if (!tgid) {
        return;
    }

    struct pid_discarder_t key = {
        .tgid = tgid,
    };

    struct discarder_params_t *params = bpf_map_lookup_elem(&pid_discarders, &key);
    if (params) {
        u64 retention;
        LOAD_CONSTANT("discarder_retention", retention);

        params->is_retained = 1;
        params->expire_at = bpf_ktime_get_ns() + retention;
    }
}

#endif
