#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "noisy-neighbor-kern-user.h"
#include "bpf_metadata.h"
#include "bpf_telemetry.h"

// Note on PID vs TID:
// In eBPF/kernel space, task_struct->pid is the Thread ID (TID)
// In userspace, this is typically called TID, while PID refers to the process group (TGID)
// We use "pid" in kernel code to match kernel conventions, but the userspace interprets it as TID

#define MAX_TASK_ENTRIES 4096
#define RATE_LIMIT_NS 100000

#define TASK_RUNNING 0

struct {
    __uint(type, BPF_MAP_TYPE_TASK_STORAGE);
    __uint(map_flags, BPF_F_NO_PREALLOC);
    __type(key, int);
    __type(value, u64);
} runq_enqueued SEC(".maps");

BPF_RINGBUF_MAP(runq_events, 0)
BPF_PERCPU_HASH_MAP(cgroup_id_to_last_event_ts, __u64, __u64, MAX_TASK_ENTRIES)
BPF_PERCPU_HASH_MAP(cgroup_agg_stats, __u64, cgroup_agg_stats_t, MAX_TASK_ENTRIES)
BPF_HASH_MAP(cgroup_pids, pid_key_t, __u8, 10000)

void bpf_rcu_read_lock(void) __ksym;
void bpf_rcu_read_unlock(void) __ksym;

u64 get_task_cgroup_id(struct task_struct *task) {
    struct css_set *cgroups;
    u64 cgroup_id;
    bpf_rcu_read_lock();
    cgroups = task->cgroups;
    cgroup_id = cgroups->dfl_cgrp->kn->id;
    bpf_rcu_read_unlock();
    return cgroup_id;
}

static __always_inline int enqueue_timestamp(struct task_struct *task) {
    u32 pid = task->pid;
    if (!pid) {
        return 0;
    }

    u64 *ptr = bpf_task_storage_get(&runq_enqueued, task, 0, BPF_LOCAL_STORAGE_GET_F_CREATE);
    if (!ptr) {
        return 0;
    }
    *ptr = bpf_ktime_get_ns();
    return 0;
}

SEC("tp_btf/sched_wakeup")
int tp_sched_wakeup(u64 *ctx) {
    struct task_struct *task = (void *)ctx[0];
    return enqueue_timestamp(task);
}

SEC("tp_btf/sched_wakeup_new")
int tp_sched_wakeup_new(u64 *ctx) {
    struct task_struct *task = (void *)ctx[0];
    return enqueue_timestamp(task);
}

SEC("tp_btf/sched_switch")
int tp_sched_switch(u64 *ctx) {
    struct task_struct *prev = (struct task_struct *)ctx[1];
    struct task_struct *next = (struct task_struct *)ctx[2];
    if (prev->__state == TASK_RUNNING) {
        enqueue_timestamp(prev);
    }

    u32 prev_pid = prev->pid;
    u32 next_pid = next->pid;

    u64 prev_cgroup_id = get_task_cgroup_id(prev);

    if (prev_pid != 0 && next_pid == 0 && prev->__state == TASK_RUNNING) {
        cgroup_agg_stats_t *stats = bpf_map_lookup_elem(&cgroup_agg_stats, &prev_cgroup_id);
        if (!stats) {
            cgroup_agg_stats_t zero = {};
            bpf_map_update_with_telemetry(cgroup_agg_stats, &prev_cgroup_id, &zero, BPF_NOEXIST);
            stats = bpf_map_lookup_elem(&cgroup_agg_stats, &prev_cgroup_id);
            // Populate cgroup name on first creation
            if (stats) {
                bpf_rcu_read_lock();
                bpf_probe_read_kernel_str_with_telemetry(stats->cgroup_name, sizeof(stats->cgroup_name),
                                                          prev->cgroups->dfl_cgrp->kn->name);
                bpf_rcu_read_unlock();
            }
        }
        if (stats) {
            __sync_fetch_and_add(&stats->preemption_count, 1);
        }
    }

    if (!next_pid) {
        return 0;
    }

    // fetch timestamp of when the next task was enqueued
    u64 *tsp = bpf_task_storage_get(&runq_enqueued, next, 0, 0);
    if (tsp == NULL) {
        return 0; // missed enqueue
    }

    // calculate runq latency before deleting the stored timestamp
    u64 now = bpf_ktime_get_ns();
    u64 runq_lat = now - *tsp;

    // delete pid from enqueued map
    bpf_task_storage_delete(&runq_enqueued, next);

    u64 cgroup_id = get_task_cgroup_id(next);

    cgroup_agg_stats_t *stats = bpf_map_lookup_elem(&cgroup_agg_stats, &cgroup_id);
    if (!stats) {
        cgroup_agg_stats_t zero = {};
        bpf_map_update_with_telemetry(cgroup_agg_stats, &cgroup_id, &zero, BPF_NOEXIST);
        stats = bpf_map_lookup_elem(&cgroup_agg_stats, &cgroup_id);
        if (stats) {
            bpf_rcu_read_lock();
            bpf_probe_read_kernel_str_with_telemetry(stats->cgroup_name, sizeof(stats->cgroup_name),
                                                      next->cgroups->dfl_cgrp->kn->name);
            bpf_rcu_read_unlock();
        }
    }
    if (stats) {
        __sync_fetch_and_add(&stats->sum_latencies_ns, runq_lat);
        __sync_fetch_and_add(&stats->event_count, 1);
    }

    pid_key_t pid_key = {.cgroup_id = cgroup_id, .pid = next_pid};
    __u8 pid_marker = 1;
    bpf_map_update_elem(&cgroup_pids, &pid_key, &pid_marker, BPF_ANY);

    // === LEGACY: Ringbuffer events (rate-limited for backwards compatibility) ===
    // Rate limit: max 1 event per cgroup per 100ms per CPU
    // Provides individual samples while aggregation provides comprehensive averages
    // per-cgroup-id-per-CPU rate-limiting to balance observability with performance
    u64 *last_ts = bpf_map_lookup_elem(&cgroup_id_to_last_event_ts, &cgroup_id);
    u64 last_ts_val = last_ts == NULL ? 0 : *last_ts;

    // check the rate limit for the cgroup_id in consideration
    // before doing more work
    if (now - last_ts_val < RATE_LIMIT_NS) {
        // Rate limit exceeded, drop the event
        return 0;
    }

    runq_event_t *event = bpf_ringbuf_reserve_with_telemetry(&runq_events, sizeof(*event), 0);
    if (!event) {
        return 0;
    }

    event->prev_cgroup_id = prev_cgroup_id;
    event->cgroup_id = cgroup_id;
    event->runq_lat = runq_lat;
    event->ts = now;
    event->pid = next_pid;
    event->prev_pid = prev_pid;

    // read cgroup names
    bpf_rcu_read_lock();
    bpf_probe_read_kernel_str_with_telemetry(event->prev_cgroup_name, sizeof(event->prev_cgroup_name), prev->cgroups->dfl_cgrp->kn->name);
    bpf_probe_read_kernel_str_with_telemetry(event->cgroup_name, sizeof(event->cgroup_name), next->cgroups->dfl_cgrp->kn->name);
    bpf_rcu_read_unlock();

    bpf_ringbuf_submit(event, 0);
    // Update the last event timestamp for the current cgroup_id
    bpf_map_update_with_telemetry(cgroup_id_to_last_event_ts, &cgroup_id, &now, BPF_ANY);

    return 0;
}

char _license[] SEC("license") = "GPL";
