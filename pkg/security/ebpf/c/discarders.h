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

void * __attribute__((always_inline)) is_discarded(struct bpf_map_def *discarder_map, void *key, u64 event_type) {
    void *entry = bpf_map_lookup_elem(discarder_map, key);
    if (entry == NULL)
        return NULL;

    struct discarder_params_t *params = (struct discarder_params_t *)entry;

    u64 tm = bpf_ktime_get_ns();

    // this discarder has been marked as on hold by event such as unlink, rename, etc.
    // keep them for a while in the map to avoid userspace to reinsert it with a pending userspace event
    if (params->is_retained) {
        if (params->expire_at < tm) {
            bpf_map_delete_elem(discarder_map, key);
        }
        return NULL;
    }

    u64* pid_tm = get_discarder_timestamp(params, event_type);
    if (pid_tm != NULL && *pid_tm && *pid_tm <= tm) {
        return NULL;
    }

    if (mask_has_event(params->event_mask, event_type)) {
        return entry;
    }

    return NULL;
}

// do not remove it directly, first mark it as on hold for a period of time, after that it will be removed
void __attribute__((always_inline)) remove_discarder(struct bpf_map_def *discarder_map, void *key) {
    struct discarder_params_t *params = bpf_map_lookup_elem(discarder_map, key);
    if (params) {
        u64 retention;
        LOAD_CONSTANT("discarder_retention", retention);

        params->is_retained = 1;
        params->expire_at = bpf_ktime_get_ns() + retention;
    }
}

struct inode_discarder_t {
    struct path_key_t path_key;
    u32 is_leaf;
    u32 padding;
};

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

    struct inode_discarder_params_t *inode_params = bpf_map_lookup_elem(&inode_discarders, &key);
    if (inode_params) {
        inode_params->params.event_mask |= 1 << (event_type - EVENT_FIRST_DISCARDER);
        add_event_to_mask(&inode_params->params.event_mask, event_type);

        if ((discarder_timestamp = get_discarder_timestamp(&inode_params->params, event_type)) != NULL) {
            *discarder_timestamp = timestamp;
        }

        u64 tm = bpf_ktime_get_ns();
        if (inode_params->params.is_retained && inode_params->params.expire_at < tm) {
            inode_params->params.is_retained = 0;
        }
    } else {
        struct inode_discarder_params_t new_inode_params = {
            .revision = get_discarder_revision(mount_id),
        };
        add_event_to_mask(&new_inode_params.params.event_mask, event_type);

        if ((discarder_timestamp = get_discarder_timestamp(&new_inode_params.params, event_type)) != NULL) {
            *discarder_timestamp = timestamp;
        }
        bpf_map_update_elem(&inode_discarders, &key, &new_inode_params, BPF_NOEXIST);
    }

    return 0;
}

struct is_discarded_by_inode_t {
    u64 event_type;
    u64 inode;
    u32 mount_id;
    u32 is_leaf;

    u64 now;
    u32 tgid_is_traced;
    u64 tgid_exec_ts;
    u32 tgid;
};

int __attribute__((always_inline)) is_discarded_by_inode(struct is_discarded_by_inode_t *params) {
    if (params->tgid_is_traced) {
        struct traced_inode_t traced_key = {
            .tgid = params->tgid,
            .inode = params->inode,
            .mount_id = params->mount_id,
        };
        u64 *last_sent = bpf_map_lookup_elem(&traced_inodes, &traced_key);
        if (last_sent == NULL) {
            bpf_map_update_elem(&traced_inodes, &traced_key, &params->now, BPF_ANY);
            return 0;
        } else {
            if (params->tgid_exec_ts > *last_sent || params->tgid_exec_ts == 0) {
                // the tgid was reused, update the last_sent timestamp and do not discard
                *last_sent = params->now;
                return 0;
            } else {
                return 1;
            }
        }
    }

    struct inode_discarder_t key = {
        .path_key = {
            .ino = params->inode,
            .mount_id = params->mount_id,
        },
        .is_leaf = params->is_leaf,
    };

    struct inode_discarder_params_t *inode_params = (struct inode_discarder_params_t *) is_discarded(&inode_discarders, &key, params->event_type);
    if (!inode_params) {
        return 0;
    }

    return inode_params->revision == get_discarder_revision(params->mount_id);
}

void __attribute__((always_inline)) remove_inode_discarders(u32 mount_id, u64 inode) {
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
        .revision = get_discarder_revision(mount_id),
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

int __attribute__((always_inline)) expire_inode_discarder(u32 mount_id, u64 inode) {
    if (!mount_id || !inode) {
        return 0;
    }

    remove_inode_discarders(mount_id, inode);
    return 0;
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

        if ((discarder_timestamp = get_discarder_timestamp(&pid_params->params, event_type)) != NULL) {
            *discarder_timestamp = timestamp;
        }
    } else {
        struct pid_discarder_params_t new_pid_params = {};
        add_event_to_mask(&new_pid_params.params.event_mask, event_type);

        if ((discarder_timestamp = get_discarder_timestamp(&new_pid_params.params, event_type)) != NULL) {
            *discarder_timestamp = timestamp;
        }
        bpf_map_update_elem(&pid_discarders, &key, &new_pid_params, BPF_NOEXIST);
    }

    return 0;
}

int __attribute__((always_inline)) is_discarded_by_pid(u64 event_type, u32 tgid) {
    struct pid_discarder_t key = {
        .tgid = tgid,
    };

    return is_discarded(&pid_discarders, &key, event_type) != NULL;
}

int __attribute__((always_inline)) is_discarded_by_process(const char mode, u64 event_type) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;

    u64 runtime_pid;
    LOAD_CONSTANT("runtime_pid", runtime_pid);

    if (is_runtime_discarded() && runtime_pid == tgid) {
        return 1;
    }

    if (mode != NO_FILTER) {
        // try with pid first
        if (is_discarded_by_pid(event_type, tgid))
            return 1;

        struct proc_cache_t *entry = get_proc_cache(tgid);
        struct is_discarded_by_inode_t params = {
            .event_type = event_type,
            .inode = entry->executable.path_key.ino,
            .mount_id = entry->executable.path_key.mount_id,
        };
        if (entry && is_discarded_by_inode(&params)) {
            return 1;
        }
    }

    return 0;
}

void __attribute__((always_inline)) remove_pid_discarder(u32 tgid) {
    struct pid_discarder_t key = {
        .tgid = tgid,
    };

    remove_discarder(&pid_discarders, &key);
}

#endif
