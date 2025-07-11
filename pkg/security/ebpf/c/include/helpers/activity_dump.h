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
#include "rate_limiter.h"

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

__attribute__((always_inline)) u32 is_cgroup_activity_dumps_supported(struct cgroup_context_t *cgroup) {
    u32 cgroup_manager = cgroup->cgroup_flags & CGROUP_MANAGER_MASK;
    u32 supported = (cgroup_manager != CGROUP_MANAGER_UNDEFINED) && (bpf_map_lookup_elem(&activity_dump_config_defaults, &cgroup_manager) != NULL);
    return supported;
}

__attribute__((always_inline)) bool reserve_traced_cgroup_spot(struct cgroup_context_t *cgroup, u64 now, u64 cookie, struct activity_dump_config *config) {
    // insert dump config defaults
    u32 cgroup_flags = cgroup->cgroup_flags;
    struct activity_dump_config *defaults = bpf_map_lookup_elem(&activity_dump_config_defaults, &cgroup_flags);
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

    struct path_key_t path_key;
    path_key = cgroup->cgroup_file;
    ret = bpf_map_update_elem(&traced_cgroups, &path_key, &cookie, BPF_NOEXIST);
    if (ret < 0) {
        // we didn't get a lock, skip this cgroup for now and go back to it later
        bpf_map_delete_elem(&activity_dumps_config, &cookie);
        return false;
    }

    // we're tracing a new cgroup, update its wait list timeout
    bpf_map_update_elem(&cgroup_wait_list, &path_key, &config->wait_list_timestamp, BPF_ANY);
    return true;
}

__attribute__((always_inline)) u64 trace_new_cgroup(void *ctx, u64 now, struct container_context_t *container) {
    u64 cookie = rand64();
    struct activity_dump_config config = {};

    if (!reserve_traced_cgroup_spot(&container->cgroup_context, now, cookie, &config)) {
        // we're already tracing too many cgroups concurrently, ignore this one for now
        return 0;
    }

    // send cgroup tracing event
    struct cgroup_tracing_event_t *evt = get_cgroup_tracing_event();
    if (evt == NULL) {
        // should never happen, ignore
        return 0;
    }

    if (!is_cgroup_activity_dumps_supported(&container->cgroup_context)) {
        return 0;
    }

    if ((container->cgroup_context.cgroup_flags&CGROUP_MANAGER_MASK) != CGROUP_MANAGER_SYSTEMD) {
        copy_container_id(container->container_id, evt->container.container_id);
    } else {
        evt->container.container_id[0] = '\0';
    }
    evt->container.cgroup_context = container->cgroup_context;
    evt->cookie = cookie;
    evt->config = config;
    evt->pid = bpf_get_current_pid_tgid() >> 32;
    send_event_ptr(ctx, EVENT_CGROUP_TRACING, evt);

    return cookie;
}

__attribute__((always_inline)) u64 should_trace_new_process_cgroup(void *ctx, u64 now, u32 pid, struct container_context_t *container) {
    // should we start tracing this cgroup ?
    struct cgroup_context_t cgroup_context;
    bpf_probe_read(&cgroup_context, sizeof(cgroup_context), &container->cgroup_context);

    if (is_cgroup_activity_dumps_enabled() && is_cgroup_activity_dumps_supported(&cgroup_context)) {
        // is this cgroup traced ?
        u64 *cookie = bpf_map_lookup_elem(&traced_cgroups, &cgroup_context.cgroup_file);

        if (cookie) {
            u64 cookie_val = *cookie;
            struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
            if (config == NULL) {
                // delete expired cgroup entry
                bpf_map_delete_elem(&traced_cgroups, &cgroup_context.cgroup_file);
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
                bpf_map_delete_elem(&traced_cgroups, &cgroup_context.cgroup_file);
                // delete config
                bpf_map_delete_elem(&activity_dumps_config, &cookie_val);
                return 0;
            }

            // We're still tracing this cgroup, update the pid cookie
            bpf_map_update_elem(&traced_pids, &pid, &cookie_val, BPF_ANY);
            return cookie_val;

        } else {
            // have we seen this cgroup before ?
            u64 *wait_timeout = bpf_map_lookup_elem(&cgroup_wait_list, &cgroup_context.cgroup_file);
            if (wait_timeout) {
                if (now > *wait_timeout) {
                    // delete expired wait_list entry
                    bpf_map_delete_elem(&cgroup_wait_list, &cgroup_context.cgroup_file);
                }

                // this cgroup is on the wait list, do not start tracing it
                return 0;
            }

            // can we start tracing this cgroup ?
            u64 cookie_val = trace_new_cgroup(ctx, now, container);
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

__attribute__((always_inline)) u64 should_trace_new_process(void *ctx, u64 now, u32 pid, struct container_context_t* container) {
    u64 cookie = should_trace_new_process_cgroup(ctx, now, pid, container);

    return cookie;
}

__attribute__((always_inline)) void inherit_traced_state(void *ctx, u32 ppid, u32 pid, struct container_context_t *container) {
    u64 now = bpf_ktime_get_ns();

    // check if the parent is traced, update the child timeout if need be
    u64 *ppid_cookie = bpf_map_lookup_elem(&traced_pids, &ppid);
    if (ppid_cookie == NULL) {
        // should_trace_new_process seems to check if cgroup needs to be checked which
        // may make sense in this case as we are inheriting from a traced cgroup, so
        // it may be ok to not set cgroup flags
        should_trace_new_process(ctx, now, pid, container);
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

__attribute__((always_inline)) u32 is_activity_dump_running(void *ctx, u32 pid, u64 now, u32 event_type) {
    u64 cookie = 0;
    struct activity_dump_config *config = NULL;

    struct proc_cache_t *pc = get_proc_cache(pid);
    if (pc) {
        cookie = should_trace_new_process(ctx, now, pid, &pc->container);
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

    if (!activity_dump_rate_limiter_allow(config->events_rate, cookie, now, 1)) {
        return 0;
    }

    return 1;
}

#endif
