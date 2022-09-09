#ifndef _ACTIVITY_DUMP_H_
#define _ACTIVITY_DUMP_H_

struct activity_dump_config {
    u64 event_mask;
    u64 timeout;
    u64 start_timestamp;
    u64 end_timestamp;
    u32 events_rate;
    u32 padding;
};

struct activity_dump_rate_limiter_ctx {
    u64 current_period;
    u32 counter;
    u32 padding;
};

struct bpf_map_def SEC("maps/activity_dump_rate_limiters") activity_dump_rate_limiters = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct activity_dump_rate_limiter_ctx),
    .max_entries = 1, // will be overridden at runtime
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

__attribute__((always_inline)) struct activity_dump_config *lookup_or_delete_traced_pid(u32 pid, u64 now, u32 *cookie) {
    if (cookie == NULL) {
        cookie = bpf_map_lookup_elem(&traced_pids, &pid);
    }
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

__attribute__((always_inline)) u32 should_trace_new_process_comm(void *ctx, u64 now, u32 pid, char comm[TASK_COMM_LEN]) {
    // should we start tracing this comm ?
    u32 *cookie = bpf_map_lookup_elem(&traced_comms, &comm[0]);
    if (cookie == NULL) {
        return 0;
    }

    u32 cookie_val = *cookie;
    struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
    if (config == NULL) {
        // this dump was stopped, delete comm entry
        bpf_map_delete_elem(&traced_comms, &comm[0]);
        return 0;
    }

    if (now > config->end_timestamp) {
        // remove expired dump
        bpf_map_delete_elem(&traced_comms, &comm[0]);
        bpf_map_delete_elem(&activity_dumps_config, &cookie_val);
        return 0;
    }

    // we're still tracing this comm, update the pid cookie
    bpf_map_update_elem(&traced_pids, &pid, &cookie_val, BPF_ANY);
    return cookie_val;
}

__attribute__((always_inline)) u32 should_trace_new_process_cgroup(void *ctx, u64 now, u32 pid, char cgroup[CONTAINER_ID_LEN]) {
    // should we start tracing this cgroup ?
    if (is_cgroup_activity_dumps_enabled() && cgroup[0] != 0) {

        // is this cgroup traced ?
        u32 *cookie = bpf_map_lookup_elem(&traced_cgroups, &cgroup[0]);

        if (cookie) {

            u32 cookie_val = *cookie;
            struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
            if (config == NULL) {
                // delete expired cgroup entry
                bpf_map_delete_elem(&traced_cgroups, &cgroup[0]);
                return 0;
            }

            if (now > config->end_timestamp) {
                // delete expired cgroup entry
                bpf_map_delete_elem(&traced_cgroups, &cgroup[0]);
                // delete config
                bpf_map_delete_elem(&activity_dumps_config, &cookie_val);
                return 0;
            }

            // We're still tracing this cgroup, update the pid cookie
            bpf_map_update_elem(&traced_pids, &pid, &cookie_val, BPF_ANY);
            return cookie_val;

        } else {

            // have we seen this cgroup before ?
            u64 *wait_timeout = bpf_map_lookup_elem(&cgroup_wait_list, &cgroup[0]);
            if (wait_timeout) {

                if (now > *wait_timeout) {
                    // delete expired wait_list entry
                    bpf_map_delete_elem(&cgroup_wait_list, &cgroup[0]);
                }

                // this cgroup is on the wait list, do not start tracing it
                return 0;
            }

            // can we start tracing this cgroup ?
            u32 cookie_val = trace_new_cgroup(ctx, now, cgroup);
            if (cookie_val == 0) {
                return 0;
            }
            // a lock was acquired for this cgroup, start tracing the current pid
            bpf_map_update_elem(&traced_pids, &pid, &cookie_val, BPF_ANY);
            return cookie_val;
        }
    }
    return 0;
}

#define ASSIGN_RETURN_IF_VAL_NULL(var, func) \
    if (var) func; \
    else var = func

union container_id_comm_combo {
    char container_id[CONTAINER_ID_LEN];
    char comm[TASK_COMM_LEN];
};

__attribute__((always_inline)) u32 should_trace_new_process(void *ctx, u64 now, u32 pid, char* cgroup_p, char* comm_p) {
    // prepare comm and cgroup (for compatibility with old kernels)
    union container_id_comm_combo buffer = {};

    bpf_probe_read(&buffer.container_id, sizeof(buffer.container_id), cgroup_p);
    u32 cookie = should_trace_new_process_cgroup(ctx, now, pid, buffer.container_id);

    bpf_probe_read(&buffer.comm, sizeof(buffer.comm), comm_p);
    // prioritize the cookie from the cgroup to the cookie from the comm
    ASSIGN_RETURN_IF_VAL_NULL(cookie, should_trace_new_process_comm(ctx, now, pid, buffer.comm));
    return cookie;
}

__attribute__((always_inline)) void inherit_traced_state(void *ctx, u32 ppid, u32 pid, char* cgroup_p, char* comm_p) {
    u64 now = bpf_ktime_get_ns();

    // check if the parent is traced, update the child timeout if need be
    u32 *ppid_cookie = bpf_map_lookup_elem(&traced_pids, &ppid);
    if (ppid_cookie == NULL) {
        // check if the current pid should be traced
        should_trace_new_process(ctx, now, pid, cgroup_p, comm_p);
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

__attribute__((always_inline)) u8 activity_dump_rate_limiter_allow(struct activity_dump_config *config, u32 cookie, u64 now, u8 should_count) {
    struct activity_dump_rate_limiter_ctx* rate_ctx_p = bpf_map_lookup_elem(&activity_dump_rate_limiters, &cookie);
    if (rate_ctx_p == NULL) {
        struct activity_dump_rate_limiter_ctx rate_ctx = {
            .current_period = now,
            .counter = should_count,
        };
        bpf_map_update_elem(&activity_dump_rate_limiters, &cookie, &rate_ctx, BPF_ANY);
        return 1;
    }

    if (now < rate_ctx_p->current_period) { // this should never happen, ignore
        return 0;
    }

    u64 delta = now - rate_ctx_p->current_period;
    if (delta > 1000000000) { // if more than 1 sec ellapsed we reset the period
        rate_ctx_p->current_period = now;
        rate_ctx_p->counter = should_count;
        return 1;
    }

    if (rate_ctx_p->counter >= config->events_rate) {
        return 0;
    } else if (should_count) {
        __sync_fetch_and_add(&rate_ctx_p->counter, 1);
    }
    return 1;
}

#define NO_ACTIVITY_DUMP       0
#define ACTIVITY_DUMP_RUNNING  1

__attribute__((always_inline)) u32 get_activity_dump_state(void *ctx, u32 pid, u64 now, u32 event_type) {
    u32 cookie = 0;
    struct activity_dump_config *config = NULL;

    struct proc_cache_t *pc = get_proc_cache(pid);
    if (pc) {
        cookie = should_trace_new_process(ctx, now, pid, pc->container.container_id, pc->entry.comm);
    }

    if (cookie != 0) {
        config = bpf_map_lookup_elem(&activity_dumps_config, &cookie);
    } else {
        // the proc_cache entry might have disappeared, try selecting the config with the pid directly
        config = lookup_or_delete_traced_pid(pid, now, NULL);
    }
    if (config == NULL) {
        return NO_ACTIVITY_DUMP;
    }

    // is this event type traced ?
    if (!mask_has_event(config->event_mask, event_type)) {
        return NO_ACTIVITY_DUMP;
    }

    if (!activity_dump_rate_limiter_allow(config, cookie, now, 1)) {
        return NO_ACTIVITY_DUMP;
    }

    // set ACTIVITY_DUMP_RUNNING
    return ACTIVITY_DUMP_RUNNING;
}

#endif
