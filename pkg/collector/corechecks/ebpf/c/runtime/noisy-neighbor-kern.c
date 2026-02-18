#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "noisy-neighbor-kern-user.h"
#include "bpf_metadata.h"
#include "bpf_telemetry.h"

#define MAX_TASK_ENTRIES 4096
#define TASK_RUNNING 0

struct {
    __uint(type, BPF_MAP_TYPE_TASK_STORAGE);
    __uint(map_flags, BPF_F_NO_PREALLOC);
    __type(key, int);
    __type(value, u64);
} runq_enqueued SEC(".maps");

// Aggregation maps for metrics
BPF_PERCPU_HASH_MAP(cgroup_agg_stats, __u64, cgroup_agg_stats_t, MAX_TASK_ENTRIES)
// PID tracking map
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

static __always_inline cgroup_agg_stats_t *get_or_create_cgroup_stats(u64 cgroup_id) {
    cgroup_agg_stats_t *stats = bpf_map_lookup_elem(&cgroup_agg_stats, &cgroup_id);
    if (!stats) {
        cgroup_agg_stats_t zero = {};
        bpf_map_update_with_telemetry(cgroup_agg_stats, &cgroup_id, &zero, BPF_NOEXIST);
        stats = bpf_map_lookup_elem(&cgroup_agg_stats, &cgroup_id);
    }
    return stats;
}

static __always_inline void track_pid(u64 cgroup_id, u32 pid) {
    pid_key_t key = {.cgroup_id = cgroup_id, .pid = pid};
    __u8 one = 1;
    bpf_map_update_elem(&cgroup_pids, &key, &one, BPF_ANY);
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
    u32 prev_pid = prev->pid;
    u32 next_pid = next->pid;

    if (prev->__state == TASK_RUNNING) {
        enqueue_timestamp(prev);
    }

    bool preemption = prev_pid != 0 && next_pid == 0 && prev->__state == TASK_RUNNING;
    if (preemption) {
        u64 prev_cgroup_id = get_task_cgroup_id(prev);
        cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(prev_cgroup_id);
        if (stats) {
            __sync_fetch_and_add(&stats->preemption_count, 1);
        }
    }

    if (!next_pid) {
        return 0;
    }

    u64 *tsp = bpf_task_storage_get(&runq_enqueued, next, 0, 0);
    if (!tsp) {
        return 0;
    }

    u64 runq_lat = bpf_ktime_get_ns() - *tsp;
    bpf_task_storage_delete(&runq_enqueued, next);

    u64 cgroup_id = get_task_cgroup_id(next);
    cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(cgroup_id);
    if (stats) {
        __sync_fetch_and_add(&stats->sum_latencies_ns, runq_lat);
        __sync_fetch_and_add(&stats->event_count, 1);
    }
    track_pid(cgroup_id, next_pid);

    return 0;
}

char _license[] SEC("license") = "GPL";
