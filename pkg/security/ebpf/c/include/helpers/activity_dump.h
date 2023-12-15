#ifndef _ACTIVITY_DUMP_H_
#define _ACTIVITY_DUMP_H_

#include "constants/custom.h"
#include "events_definition.h"
#include "maps.h"
#include "perf_ring.h"

#include "dentry_resolver.h"
#include "container.h"
#include "events.h"
#include "process.h"

__attribute__((always_inline)) struct activity_dump_config *lookup_or_delete_traced_pid(u32 pid, u64 now, u64 *cookie) {
    if (cookie == NULL) {
        cookie = bpf_map_lookup_elem(&traced_pids, &pid);
    }
    if (cookie == NULL) {
        return NULL;
    }

    u64 cookie_val = *cookie; // for older kernels
    struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
    if (config == NULL) {
        return NULL;
    }

    // Warning: this check has to be made before any other check on an existing config. The rational is that a dump is
    // paused by the user space load controller which will be working on resuming the dump, with updated config
    // parameters. Stopping a paused dump in kernel space (= removing its entry from traced_cgroups) can lead to a race
    // on the traced cgroups counter: the kernel might want to "restart dumping this cgroup" even if the user space load
    // controller isn't done with it.
    if (config->paused) {
        return NULL;
    }

    if (now > config->end_timestamp) {
        // delete expired entries
        bpf_map_delete_elem(&traced_pids, &pid);
        bpf_map_delete_elem(&activity_dumps_config, &cookie_val);
        return NULL;
    }
    return config;
}

__attribute__((always_inline)) struct cgroup_tracing_event_t *get_cgroup_tracing_event() {
    u32 key = bpf_get_current_pid_tgid() % EVENT_GEN_SIZE;
    struct cgroup_tracing_event_t *evt = bpf_map_lookup_elem(&cgroup_tracing_event_gen, &key);
    if (evt == NULL) {
        return 0;
    }
    evt->container.container_id[0] = 0;
    return evt;
}

__attribute__((always_inline)) bool reserve_traced_cgroup_spot(char cgroup[CONTAINER_ID_LEN], u64 now, u64 cookie, struct activity_dump_config *config) {
    // insert dump config defaults
    u32 defaults_key = 0;
    struct activity_dump_config *defaults = bpf_map_lookup_elem(&activity_dump_config_defaults, &defaults_key);
    if (defaults == NULL) {
        // should never happen, ignore
        return false;
    }
    *config = *defaults;
    config->start_timestamp = now;
    config->end_timestamp = config->start_timestamp + config->timeout;
    config->wait_list_timestamp = config->start_timestamp + config->wait_list_timestamp;

    int ret = bpf_map_update_elem(&activity_dumps_config, &cookie, config, BPF_ANY);
    if (ret < 0) {
        // should never happen, ignore
        return false;
    }

    ret = bpf_map_update_elem(&traced_cgroups, &cgroup[0], &cookie, BPF_NOEXIST);
    if (ret < 0) {
        // we didn't get a lock, skip this cgroup for now and go back to it later
        bpf_map_delete_elem(&activity_dumps_config, &cookie);
        return false;
    }

    // we're tracing a new cgroup, update its wait list timeout
    bpf_map_update_elem(&cgroup_wait_list, &cgroup[0], &config->wait_list_timestamp, BPF_ANY);
    return true;
}

__attribute__((always_inline)) u64 trace_new_cgroup(void *ctx, u64 now, char cgroup[CONTAINER_ID_LEN]) {
    u64 cookie = rand64();
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

__attribute__((always_inline)) u64 should_trace_new_process_comm(void *ctx, u64 now, u32 pid, char comm[TASK_COMM_LEN]) {
    // should we start tracing this comm ?
    u64 *cookie = bpf_map_lookup_elem(&traced_comms, &comm[0]);
    if (cookie == NULL) {
        return 0;
    }

    u64 cookie_val = *cookie;
    struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
    if (config == NULL) {
        // this dump was stopped, delete comm entry
        bpf_map_delete_elem(&traced_comms, &comm[0]);
        return 0;
    }

    // Warning: this check has to be made before any other check on an existing config. The rational is that a dump is
    // paused by the user space load controller which will be working on resuming the dump, with updated config
    // parameters. Stopping a paused dump in kernel space (= removing its entry from traced_cgroups) can lead to a race
    // on the traced cgroups counter: the kernel might want to "restart dumping this cgroup" even if the user space load
    // controller isn't done with it.
    if (config->paused) {
        // ignore for now, the userspace load controller will re-enable this dump soon
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

__attribute__((always_inline)) u64 should_trace_new_process_cgroup(void *ctx, u64 now, u32 pid, char cgroup[CONTAINER_ID_LEN]) {
    // should we start tracing this cgroup ?
    if (is_cgroup_activity_dumps_enabled() && cgroup[0] != 0) {

        // is this cgroup traced ?
        u64 *cookie = bpf_map_lookup_elem(&traced_cgroups, &cgroup[0]);

        if (cookie) {

            u64 cookie_val = *cookie;
            struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
            if (config == NULL) {
                // delete expired cgroup entry
                bpf_map_delete_elem(&traced_cgroups, &cgroup[0]);
                return 0;
            }

            // Warning: this check has to be made before any other check on an existing config. The rational is that a dump is
            // paused by the user space load controller which will be working on resuming the dump, with updated config
            // parameters. Stopping a paused dump in kernel space (= removing its entry from traced_cgroups) can lead to a race
            // on the traced cgroups counter: the kernel might want to "restart dumping this cgroup" even if the user space load
            // controller isn't done with it.
            if (config->paused) {
                // ignore for now, the userspace load controller will re-enable this dump soon
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
            u64 cookie_val = trace_new_cgroup(ctx, now, cgroup);
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

union container_id_comm_combo {
    char container_id[CONTAINER_ID_LEN];
    char comm[TASK_COMM_LEN];
};

__attribute__((always_inline)) u64 should_trace_new_process(void *ctx, u64 now, u32 pid, char* cgroup_p, char* comm_p) {
    // prepare comm and cgroup (for compatibility with old kernels)
    union container_id_comm_combo buffer = {};

    bpf_probe_read(&buffer.container_id, sizeof(buffer.container_id), cgroup_p);
    u64 cookie = should_trace_new_process_cgroup(ctx, now, pid, buffer.container_id);

    // prioritize the cookie from the cgroup to the cookie from the comm
    if (!cookie) {
        bpf_probe_read(&buffer.comm, sizeof(buffer.comm), comm_p);
        cookie = should_trace_new_process_comm(ctx, now, pid, buffer.comm);
    }
    return cookie;
}

__attribute__((always_inline)) void inherit_traced_state(void *ctx, u32 ppid, u32 pid, char* cgroup_p, char* comm_p) {
    u64 now = bpf_ktime_get_ns();

    // check if the parent is traced, update the child timeout if need be
    u64 *ppid_cookie = bpf_map_lookup_elem(&traced_pids, &ppid);
    if (ppid_cookie == NULL) {
        // check if the current pid should be traced
        should_trace_new_process(ctx, now, pid, cgroup_p, comm_p);
        return;
    }

    u64 cookie_val = *ppid_cookie;
    struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
    if (config == NULL) {
        // delete expired entries
        bpf_map_delete_elem(&traced_pids, &ppid);
        return;
    }

    // Warning: this check has to be made before any other check on an existing config. The rational is that a dump is
    // paused by the user space load controller which will be working on resuming the dump, with updated config
    // parameters. Stopping a paused dump in kernel space (= removing its entry from traced_cgroups) can lead to a race
    // on the traced cgroups counter: the kernel might want to "restart dumping this cgroup" even if the user space load
    // controller isn't done with it.
    if (config->paused) {
        // ignore for now, the userspace load controller will re-enable this dump soon
        return;
    }

    if (now > config->end_timestamp) {
        // delete expired entries
        bpf_map_delete_elem(&traced_pids, &ppid);
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

enum rate_limiter_algo_ids {
    RL_ALGO_BASIC = 0,
    RL_ALGO_BASIC_HALF,
    RL_ALGO_DECREASING_DROPRATE,
    RL_ALGO_INCREASING_DROPRATE,
    RL_ALGO_TOTAL_NUMBER,
};

__attribute__((always_inline)) u8 activity_dump_rate_limiter_reset_period(u64 now, struct activity_dump_rate_limiter_ctx* rate_ctx_p) {
    rate_ctx_p->current_period = now;
    rate_ctx_p->counter = 0;
#ifndef __BALOUM__ // do not change algo during unit tests
    rate_ctx_p->algo_id = now % RL_ALGO_TOTAL_NUMBER;
#endif /* __BALOUM__ */
    return 1;
}

__attribute__((always_inline)) u8 activity_dump_rate_limiter_allow_basic(struct activity_dump_config *config, u64 now, struct activity_dump_rate_limiter_ctx* rate_ctx_p, u64 delta) {
    if (delta > 1000000000) { // if more than 1 sec ellapsed we reset the period
        return activity_dump_rate_limiter_reset_period(now, rate_ctx_p);
    }

    if (rate_ctx_p->counter >= config->events_rate) { // if we already allowed more than rate
        return 0;
    } else {
        return 1;
    }
}

__attribute__((always_inline)) u8 activity_dump_rate_limiter_allow_basic_half(struct activity_dump_config *config, u64 now, struct activity_dump_rate_limiter_ctx* rate_ctx_p, u64 delta) {
    if (delta > 1000000000 / 2) { // if more than 0.5 sec ellapsed we reset the period
        return activity_dump_rate_limiter_reset_period(now, rate_ctx_p);
    }

    if (rate_ctx_p->counter >= config->events_rate / 2) { // if we already allowed more than rate / 2
        return 0;
    } else {
        return 1;
    }
}

__attribute__((always_inline)) u8 activity_dump_rate_limiter_allow_decreasing_droprate(struct activity_dump_config *config, u64 now, struct activity_dump_rate_limiter_ctx* rate_ctx_p, u64 delta) {
    if (delta > 1000000000) { // if more than 1 sec ellapsed we reset the period
        return activity_dump_rate_limiter_reset_period(now, rate_ctx_p);
    }

    if (rate_ctx_p->counter >= config->events_rate) { // if we already allowed more than rate
        return 0;
    } else if (rate_ctx_p->counter < (config->events_rate / 4)) { // first 1/4 is not rate limited
        return 1;
    }

    // if we are between rate / 4 and rate, apply a decreasing rate of:
    // (counter * 100) / (rate) %
    else if (now % ((rate_ctx_p->counter * 100) / config->events_rate) == 0) {
        return 1;
    }
    return 0;
}

__attribute__((always_inline)) u8 activity_dump_rate_limiter_allow_increasing_droprate(struct activity_dump_config *config, u64 now, struct activity_dump_rate_limiter_ctx* rate_ctx_p, u64 delta) {
    if (delta > 1000000000) { // if more than 1 sec ellapsed we reset the period
        return activity_dump_rate_limiter_reset_period(now, rate_ctx_p);
    }

    if (rate_ctx_p->counter >= config->events_rate) { // if we already allowed more than rate
        return 0;
    } else if (rate_ctx_p->counter < (config->events_rate / 4)) { // first 1/4 is not rate limited
        return 1;
    }

    // if we are between rate / 4 and rate, apply an increasing rate of:
    // 100 - ((counter * 100) / (rate)) %
    else if (now % (100 - ((rate_ctx_p->counter * 100) / config->events_rate)) == 0) {
        return 1;
    }
    return 0;
}

__attribute__((always_inline)) u8 activity_dump_rate_limiter_allow(struct activity_dump_config *config, u64 cookie, u64 now, u8 should_count) {
    struct activity_dump_rate_limiter_ctx* rate_ctx_p = bpf_map_lookup_elem(&activity_dump_rate_limiters, &cookie);
    if (rate_ctx_p == NULL) {
        struct activity_dump_rate_limiter_ctx rate_ctx = {
            .current_period = now,
            .counter = should_count,
            .algo_id = now % RL_ALGO_TOTAL_NUMBER,
        };
        bpf_map_update_elem(&activity_dump_rate_limiters, &cookie, &rate_ctx, BPF_ANY);
        return 1;
    }

    if (now < rate_ctx_p->current_period) { // this should never happen, ignore
        return 0;
    }
    u64 delta = now - rate_ctx_p->current_period;

    u8 allow;
    switch (rate_ctx_p->algo_id) {
    case RL_ALGO_BASIC:
        allow = activity_dump_rate_limiter_allow_basic(config, now, rate_ctx_p, delta);
        break;
    case RL_ALGO_BASIC_HALF:
        allow = activity_dump_rate_limiter_allow_basic_half(config, now, rate_ctx_p, delta);
        break;
    case RL_ALGO_DECREASING_DROPRATE:
        allow = activity_dump_rate_limiter_allow_decreasing_droprate(config, now, rate_ctx_p, delta);
        break;
    case RL_ALGO_INCREASING_DROPRATE:
        allow = activity_dump_rate_limiter_allow_increasing_droprate(config, now, rate_ctx_p, delta);
        break;
    default: // should never happen, ignore
        return 0;
    }

    if (allow && should_count) {
        __sync_fetch_and_add(&rate_ctx_p->counter, 1);
    }
    return (allow);
}

__attribute__((always_inline)) u32 is_activity_dump_running(void *ctx, u32 pid, u64 now, u32 event_type) {
    u64 cookie = 0;
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
        return 0;
    }

    // Warning: this check has to be made before any other check on an existing config. The rational is that a dump is
    // paused by the user space load controller which will be working on resuming the dump, with updated config
    // parameters. Stopping a paused dump in kernel space (= removing its entry from traced_cgroups) can lead to a race
    // on the traced cgroups counter: the kernel might want to "restart dumping this cgroup" even if the user space load
    // controller isn't done with it.
    if (config->paused) {
        // ignore for now, the userspace load controller will re-enable this dump soon
        return 0;
    }

    // is this event type traced ?
    if (!mask_has_event(config->event_mask, event_type)) {
        return 0;
    }

    if (!activity_dump_rate_limiter_allow(config, cookie, now, 1)) {
        return 0;
    }

    return 1;
}

#endif
