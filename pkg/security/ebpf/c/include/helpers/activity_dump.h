#ifndef _ACTIVITY_DUMP_H_
#define _ACTIVITY_DUMP_H_

#include "constants/custom.h"
#include "events_definition.h"
#include "maps.h"
#include "perf_ring.h"

#include "dentry_resolver.h"
#include "events.h"
#include "process.h"
#include "rate_limiter.h"
#include "helpers/utils.h"

// cgroup_mount_id map special values:
#define CGROUP_MOUNT_ID_UNSET 0 // initial value at startup, until it got set by cgroup manager
#define CGROUP_MOUNT_ID_NO_FILTER 0xFFFFFFFF // UINT32_MAX, used for cgroup v2 where we don't have to filter
// otherwise for cgroupv1 we specify the pids cgroup mount id

static __attribute__((always_inline)) u32 get_cgroup_mount_id_filter(void) {
    // Retrieve the cgroup mount id to filter on
    u32 key = 0;
    u32 *cgroup_mount_id_filter = (u32*)bpf_map_lookup_elem(&cgroup_mount_id, &key);
    if (cgroup_mount_id_filter == NULL) {
        return CGROUP_MOUNT_ID_UNSET;
    }
    return *cgroup_mount_id_filter;
}

static __attribute__((always_inline)) bool is_cgroup_mount_id_filter_valid(u32 cgroup_filter, struct path_key_t *key) {
    if (cgroup_filter == CGROUP_MOUNT_ID_UNSET) {
        return false;
    }

    // handle special case for CENTOS7 where we can't retrieve the mount_id of traced cgroup
    if (key->mount_id == 0) {
        if (key->ino == 0) {
            // can happen on CentOS7 with systemd pid 1 being not part of any cgroup
            return false;
        }

        u32 cgroup_write_type = get_cgroup_write_type();
        if (cgroup_write_type == CGROUP_CENTOS_7) {
            return true;
        } else {
            return false;
        }
    }

    if (cgroup_filter != CGROUP_MOUNT_ID_NO_FILTER && cgroup_filter != key->mount_id) {
        return false;
    }
    return true;
}

// cleanup_expired_dump removes all kernel space entries for an expired dump
// If pid is non-zero, also removes that specific PID from traced_pids
// Note: complete traced_pids cleanup requires userspace intervention since we can't iterate the map efficiently
static __attribute__((always_inline)) void cleanup_expired_dump(u64 *cgroup_inode, u64 cookie, u32 pid) {
    bpf_map_delete_elem(&activity_dumps_config, &cookie);
    if (cgroup_inode != NULL && *cgroup_inode != 0) {
        bpf_map_delete_elem(&traced_cgroups, cgroup_inode);
    }
    if (pid != 0) {
        bpf_map_delete_elem(&traced_pids, &pid);
    }
}

static __attribute__((always_inline)) struct activity_dump_config *lookup_or_delete_traced_pid(u32 pid, u64 now, u64 *cookie) {
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
        // delete expired dump and the traced pid
        cleanup_expired_dump(NULL, cookie_val, pid);
        return NULL;
    }
    return config;
}

static __attribute__((always_inline)) struct cgroup_tracing_event_t *get_cgroup_tracing_event() {
    u32 key = bpf_get_current_pid_tgid() % EVENT_GEN_SIZE;
    return bpf_map_lookup_elem(&cgroup_tracing_event_gen, &key);
}

static __attribute__((always_inline)) bool reserve_traced_cgroup_spot(struct cgroup_context_t *cgroup, u64 now, u64 cookie, struct activity_dump_config *config) {
    // get dump config defaults
    u32 empty_key = 0;
    struct activity_dump_config *defaults = bpf_map_lookup_elem(&activity_dump_config_defaults, &empty_key);
    if (defaults == NULL) {
        // should never happen, ignore
        return false;
    }

    u64 cgroup_inode = cgroup->cgroup_file.ino;
    int ret = bpf_map_update_elem(&traced_cgroups, &cgroup_inode, &cookie, BPF_NOEXIST);
    if (ret < 0) {
        // we didn't get a lock, skip this cgroup for now and go back to it later
        return false;
    }

    *config = *defaults;
    config->start_timestamp = now;
    config->end_timestamp = config->start_timestamp + config->timeout;
    config->wait_list_timestamp = config->start_timestamp + config->wait_list_timestamp;
    ret = bpf_map_update_elem(&activity_dumps_config, &cookie, config, BPF_ANY);
    if (ret < 0) {
        // should never happen, ignore
        bpf_map_delete_elem(&traced_cgroups, &cgroup_inode);
        return false;
    }

    // we're tracing a new cgroup, update its wait list timeout
    bpf_map_update_elem(&cgroup_wait_list, &cgroup_inode, &config->wait_list_timestamp, BPF_ANY);
    return true;
}

static __attribute__((always_inline)) u64 trace_new_cgroup(void *ctx, u64 now, struct cgroup_context_t *cgroup) {
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

    evt->cgroup = *cgroup;
    evt->cookie = cookie;
    evt->config = config;
    evt->pid = bpf_get_current_pid_tgid() >> 32;
    send_event_ptr(ctx, EVENT_CGROUP_TRACING, evt);

    return cookie;
}

static __attribute__((always_inline)) u64 should_trace_new_process_cgroup(void *ctx, u64 now, u32 pid, struct cgroup_context_t *cgroup) {
    // should we start tracing this cgroup ?

    // here to avoid an error in AL2-4.14 when tailcalled:
    // > load program: permission denied: 157: (85) call bpf_map_lookup_elem#1: R2 type=map_value expected=fp (243 line(s) omitted)
    struct cgroup_context_t cgroup_context;
    bpf_probe_read(&cgroup_context, sizeof(cgroup_context), cgroup);

    u32 cgroup_filter = get_cgroup_mount_id_filter();
    if (!is_cgroup_mount_id_filter_valid(cgroup_filter, &cgroup_context.cgroup_file)) {
        return 0;
    }

    if (is_cgroup_activity_dumps_enabled()) {

        u64 cgroup_inode = cgroup_context.cgroup_file.ino;

        // is this cgroup discarded ?
        u8 *discarded = bpf_map_lookup_elem(&traced_cgroups_discarded, &cgroup_inode);
        if (discarded != NULL) {
            return 0;
        }

        // is this cgroup traced ?
        u64 *cookie = bpf_map_lookup_elem(&traced_cgroups, &cgroup_inode);

        if (cookie) {
            u64 cookie_val = *cookie;
            struct activity_dump_config *config = bpf_map_lookup_elem(&activity_dumps_config, &cookie_val);
            if (config == NULL) {
                // delete expired cgroup entry
                bpf_map_delete_elem(&traced_cgroups, &cgroup_inode);
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
                // delete expired dump (no specific pid to clean here)
                cleanup_expired_dump(&cgroup_inode, cookie_val, 0);
                return 0;
            }

            // We're still tracing this cgroup, update the pid cookie
            bpf_map_update_elem(&traced_pids, &pid, &cookie_val, BPF_ANY);
            return cookie_val;

        } else {
            // have we seen this cgroup before ?
            u64 *wait_timeout = bpf_map_lookup_elem(&cgroup_wait_list, &cgroup_inode);
            if (wait_timeout) {
                if (now > *wait_timeout) {
                    // delete expired wait_list entry
                    bpf_map_delete_elem(&cgroup_wait_list, &cgroup_inode);
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

static __attribute__((always_inline)) void inherit_traced_state(void *ctx, u32 ppid, u32 pid, struct cgroup_context_t *cgroup) {
    u64 now = bpf_ktime_get_ns();

    // check if the parent is traced, update the child timeout if need be
    u64 *ppid_cookie = bpf_map_lookup_elem(&traced_pids, &ppid);
    if (ppid_cookie == NULL) {
        should_trace_new_process_cgroup(ctx, now, pid, cgroup);
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
        // delete expired dump and the traced parent pid
        cleanup_expired_dump(NULL, cookie_val, ppid);
        return;
    }

    // inherit parent cookie
    bpf_map_update_elem(&traced_pids, &pid, &cookie_val, BPF_ANY);
}

static __attribute__((always_inline)) void cleanup_traced_state(u32 pid) {
    // delete pid from traced_pids
    bpf_map_delete_elem(&traced_pids, &pid);
}

static __attribute__((always_inline)) u32 is_activity_dump_running(void *ctx, u32 pid, u64 now, u32 event_type) {
    u64 cookie = 0;
    struct activity_dump_config *config = NULL;

    struct proc_cache_t *pc = get_proc_cache(pid);
    if (pc) {
        cookie = should_trace_new_process_cgroup(ctx, now, pid, &pc->cgroup);
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
