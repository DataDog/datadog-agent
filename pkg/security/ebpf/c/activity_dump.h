#ifndef _ACTIVITY_DUMP_H_
#define _ACTIVITY_DUMP_H_

struct activity_dump_config {
    u64 event_mask;
    u64 timeout;
    u64 start_timestamp;
    u64 end_timestamp;
    // TODO(rate_limiter): add rate
};

struct bpf_map_def SEC("maps/activity_dumps_config") activity_dumps_config = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct activity_dump_config),
    .max_entries = 1, // will be overridden at runtime
};

struct bpf_map_def SEC("maps/activity_dump_config_defaults") activity_dump_config_defaults = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct activity_dump_config),
    .max_entries = 1,
};

struct bpf_map_def SEC("maps/traced_cgroups") traced_cgroups = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = CONTAINER_ID_LEN,
    .value_size = sizeof(u32),
    .max_entries = 1, // will be overridden at runtime
};

struct traced_cgroups_counter_t {
    u64 max;
    u64 counter;
};

struct bpf_map_def SEC("maps/traced_cgroups_counter") traced_cgroups_counter = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct traced_cgroups_counter_t),
    .max_entries = 1,
};

struct bpf_map_def SEC("maps/cgroup_wait_list") cgroup_wait_list = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = CONTAINER_ID_LEN,
    .value_size = sizeof(u64),
    .max_entries = 1, // will be overridden at runtime
};

struct bpf_map_def SEC("maps/traced_pids") traced_pids = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 8192,
};

struct bpf_map_def SEC("maps/traced_comms") traced_comms = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = TASK_COMM_LEN,
    .value_size = sizeof(u32),
    .max_entries = 200,
};

__attribute__((always_inline)) u64 is_cgroup_activity_dumps_enabled() {
    u64 cgroup_activity_dumps_enabled;
    LOAD_CONSTANT("cgroup_activity_dumps_enabled", cgroup_activity_dumps_enabled);
    return cgroup_activity_dumps_enabled != 0;
}

__attribute__((always_inline)) struct activity_dump_config *lookup_or_delete_traced_pid(u32 pid, u64 now) {
    u32 *cookie = bpf_map_lookup_elem(&traced_pids, &pid);
    if (cookie == NULL) {
        return 0;
    }

    u32 cookie_val = *cookie; // for older kernels
    struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
    if (config == NULL) {
        return 0;
    }

    if (now > config->end_timestamp) {
        // delete expired entries
        bpf_map_delete_elem(&traced_pids, &pid);
        bpf_map_delete_elem(&activity_dumps_config, &cookie_val);
        return 0;
    }
    return config;
}

struct cgroup_tracing_event_t {
    struct kevent_t event;
    struct container_context_t container;
    struct activity_dump_config config;
    u32 cookie;
};

struct bpf_map_def SEC("maps/cgroup_tracing_event_gen") cgroup_tracing_event_gen = {
    .type = BPF_MAP_TYPE_PERCPU_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct cgroup_tracing_event_t),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

__attribute__((always_inline)) struct cgroup_tracing_event_t *get_cgroup_tracing_event() {
    u32 key = 0;
    struct cgroup_tracing_event_t *evt = bpf_map_lookup_elem(&cgroup_tracing_event_gen, &key);
    if (evt == NULL) {
        return 0;
    }
    evt->container.container_id[0] = 0;
    return evt;
}

struct bpf_map_def SEC("maps/traced_cgroups_lock") traced_cgroups_lock = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
};

__attribute__((always_inline)) bool lock_cgroups_counter() {
    u32 key = 0;
    return bpf_map_update_elem(&traced_cgroups_lock, &key, &key, BPF_NOEXIST) == 0;
}

__attribute__((always_inline)) void unlock_cgroups_counter() {
    u32 key = 0;
    bpf_map_delete_elem(&traced_cgroups_lock, &key);
}

__attribute__((always_inline)) bool reserve_traced_cgroup_spot(char cgroup[CONTAINER_ID_LEN], u64 now, u32 cookie, struct activity_dump_config *config) {
    if (!lock_cgroups_counter()) {
        return false;
    }

    void *already_in = bpf_map_lookup_elem(&traced_cgroups, &cgroup[0]);
    if (already_in) {
        goto fail;
    }

    u32 key = 0;
    struct traced_cgroups_counter_t *counter = bpf_map_lookup_elem(&traced_cgroups_counter, &key);
    if (!counter) {
        goto fail;
    }

    if (counter->counter < counter->max) {
        counter->counter++;
    } else {
        goto fail;
    }

    // insert dump config defaults
    u32 defaults_key = 0;
    struct activity_dump_config *defaults = bpf_map_lookup_elem(&activity_dump_config_defaults, &defaults_key);
    if (defaults == NULL) {
        // should never happen, ignore
        goto fail;
    }
    *config = *defaults;
    config->start_timestamp = now;
    config->end_timestamp = config->start_timestamp + config->timeout;

    int ret = bpf_map_update_elem(&activity_dumps_config, &cookie, config, BPF_ANY);
    if (ret < 0) {
        // should never happen, ignore
        goto fail;
    }

    ret = bpf_map_update_elem(&traced_cgroups, &cgroup[0], &cookie, BPF_NOEXIST);
    if (ret < 0) {
        // this should be caught earlier but we're already tracing too many cgroups concurrently, ignore this one for now
        goto fail;
    }

    unlock_cgroups_counter();
    return true;

fail:
    unlock_cgroups_counter();
    return false;
}

__attribute__((always_inline)) void free_traced_cgroup_spot(char cgroup[CONTAINER_ID_LEN]) {
    if (!lock_cgroups_counter()) {
        return;
    }

    u32 *cookie = bpf_map_lookup_elem(&traced_cgroups, &cgroup[0]);
    if (cookie != NULL) {
        u32 cookie_val = *cookie;
        bpf_map_delete_elem(&activity_dumps_config, &cookie_val);
    }

    bpf_map_delete_elem(&traced_cgroups, &cgroup[0]);

    u32 key = 0;
    struct traced_cgroups_counter_t *counter = bpf_map_lookup_elem(&traced_cgroups_counter, &key);
    if (counter && counter->counter > 0) {
        counter->counter -= 1;
    }

    unlock_cgroups_counter();
}

__attribute__((always_inline)) u32 trace_new_cgroup(void *ctx, u64 now, char cgroup[CONTAINER_ID_LEN]) {
    u32 cookie = bpf_get_prandom_u32();
    struct activity_dump_config config = {};

    if (!reserve_traced_cgroup_spot(cgroup, now, cookie, &config)) {
        // we're already tracing too many cgroups concurrently, ignore this one for now
        return 0;
    }

    // send cgroup tracing event
    struct cgroup_tracing_event_t *evt = get_cgroup_tracing_event();
    if (evt == NULL) {
        // should never happen, ignore
        return 0;
    }
    copy_container_id(cgroup, evt->container.container_id);
    evt->cookie = cookie;
    evt->config = config;
    send_event_ptr(ctx, EVENT_CGROUP_TRACING, evt);

    // return cookie
    return cookie;
}

__attribute__((always_inline)) void should_trace_new_process_comm(void *ctx, u64 now, u32 pid, char comm[TASK_COMM_LEN]) {
    // should we start tracing this comm ?
    u32 *cookie = bpf_map_lookup_elem(&traced_comms, &comm[0]);
    if (cookie == NULL) {
        return;
    }

    u32 cookie_val = *cookie;
    struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
    if (config == NULL) {
        // this dump was stopped, delete comm entry
        bpf_map_delete_elem(&traced_comms, &comm[0]);
        return;
    }

    if (now > config->end_timestamp) {
        // remove expired dump
        bpf_map_delete_elem(&traced_comms, &comm[0]);
        bpf_map_delete_elem(&activity_dumps_config, &cookie_val);
        return;
    }

    // we're still tracing this comm, update the pid cookie
    bpf_map_update_elem(&traced_pids, &pid, &cookie_val, BPF_ANY);
}

__attribute__((always_inline)) void should_trace_new_process_cgroup(void *ctx, u64 now, u32 pid, char cgroup[CONTAINER_ID_LEN]) {
    // should we start tracing this cgroup ?
    if (is_cgroup_activity_dumps_enabled() && cgroup[0] != 0) {

        // is this cgroup traced ?
        u32 *cookie = bpf_map_lookup_elem(&traced_cgroups, &cgroup[0]);

        if (cookie) {

            u32 cookie_val = *cookie;
            struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
            if (config == NULL) {
                // delete expired cgroup entry
                free_traced_cgroup_spot(cgroup);
                return;
            }

            if (now > config->end_timestamp) {
                // delete expired cgroup entry
                free_traced_cgroup_spot(cgroup);
                // delete config
                bpf_map_delete_elem(&activity_dumps_config, &cookie_val);
                return;
            }

            // We're still tracing this cgroup, update the pid cookie
            bpf_map_update_elem(&traced_pids, &pid, &cookie_val, BPF_ANY);

        } else {

            // have we seen this cgroup before ?
            u64 *wait_timeout = bpf_map_lookup_elem(&cgroup_wait_list, &cgroup[0]);
            if (wait_timeout) {

                if (now > *wait_timeout) {
                    // delete expired wait_list entry
                    bpf_map_delete_elem(&cgroup_wait_list, &cgroup[0]);
                }

                // this cgroup is on the wait list, do not start tracing it
                return;
            }

            // can we start tracing this cgroup ?
            u32 cookie = trace_new_cgroup(ctx, now, cgroup);
            if (cookie != 0) {
                // a lock was acquired for this cgroup, start tracing the current pid
                bpf_map_update_elem(&traced_pids, &pid, &cookie, BPF_ANY);
            }
        }
    }
}

__attribute__((always_inline)) void should_trace_new_process(void *ctx, u64 now, u32 pid, char cgroup[CONTAINER_ID_LEN], char comm[TASK_COMM_LEN]) {
    should_trace_new_process_comm(ctx, now, pid, comm);
    should_trace_new_process_cgroup(ctx, now, pid, cgroup);
}

__attribute__((always_inline)) void inherit_traced_state(void *ctx, u32 ppid, u32 pid, char cgroup[CONTAINER_ID_LEN], char comm[TASK_COMM_LEN]) {
    u64 now = bpf_ktime_get_ns();

    // check if the parent is traced, update the child timeout if need be
    u32 *ppid_cookie = bpf_map_lookup_elem(&traced_pids, &ppid);
    if (ppid_cookie == NULL) {
        // check if the current pid should be traced
        should_trace_new_process(ctx, now, pid, cgroup, comm);
        return;
    }

    u32 cookie_val = *ppid_cookie;
    struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
    if (config == NULL) {
        // delete expired entries
        bpf_map_delete_elem(&traced_pids, &ppid);
        return;
    }
    if (now > config->end_timestamp) {
        // delete expired entries
        bpf_map_delete_elem(&traced_pids, &pid);
        bpf_map_delete_elem(&activity_dumps_config, &cookie_val);
        return;
    }

    // inherit parent cookie
    bpf_map_update_elem(&traced_pids, &pid, &cookie_val, BPF_ANY);
}

__attribute__((always_inline)) void cleanup_traced_state(u32 pid) {
    // delete pid from traced_pids
    bpf_map_delete_elem(&traced_pids, &pid);
}

#define NO_ACTIVITY_DUMP       0
#define ACTIVITY_DUMP_RUNNING  1

union container_id_comm_combo {
    char container_id[CONTAINER_ID_LEN];
    char comm[TASK_COMM_LEN];
};

__attribute__((always_inline)) u32 get_activity_dump_state(void *ctx, u32 pid, u64 now, u32 event_type) {
    struct proc_cache_t *pc = get_proc_cache(pid);
    if (pc) {
        // prepare comm and cgroup (for compatibility with old kernels)
        union container_id_comm_combo buffer = {};
        bpf_probe_read(&buffer.comm, sizeof(buffer.comm), pc->entry.comm);
        bpf_probe_read(&buffer.container_id, sizeof(buffer.container_id), pc->container.container_id);

        should_trace_new_process(ctx, now, pid, buffer.container_id, buffer.comm);
    }

    struct activity_dump_config *config = lookup_or_delete_traced_pid(pid, now);
    if (config == NULL) {
        return NO_ACTIVITY_DUMP;
    }

    // is this event type traced ?
    if (mask_has_event(config->event_mask, event_type)) {
        return NO_ACTIVITY_DUMP;
    }

    // TODO(rate_limiter): check if this event should be rate limited + add 1

    // set ACTIVITY_DUMP_RUNNING
    return ACTIVITY_DUMP_RUNNING;
}

#endif
