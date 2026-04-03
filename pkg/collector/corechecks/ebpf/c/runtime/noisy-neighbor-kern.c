#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "noisy-neighbor-kern-user.h"
#include "bpf_metadata.h"
#include "bpf_telemetry.h"

#define MAX_TASK_ENTRIES 4096
#define MAX_PREEMPTOR_ENTRIES 512
#define TASK_RUNNING 0

BPF_TASK_STORAGE_MAP(runq_enqueued, u64)

BPF_PERCPU_HASH_MAP(cgroup_agg_stats, __u64, cgroup_agg_stats_t, MAX_TASK_ENTRIES)

// watchlist_active: single-entry gate. If 0, sched_switch does minimal work.
BPF_ARRAY_MAP(watchlist_active, __u8, 1)

// watchlist: cgroup IDs that Layer 1 flagged for detailed monitoring
BPF_HASH_MAP(watchlist, __u64, __u8, 128)

// preemptor_stats: tracks which foreign cgroups preempt watched cgroups
BPF_PERCPU_HASH_MAP(preemptor_stats, preemptor_key_t, preemptor_stats_t, MAX_PREEMPTOR_ENTRIES)

void bpf_rcu_read_lock(void) __ksym;
void bpf_rcu_read_unlock(void) __ksym;
extern void *bpf_rdonly_cast(const void *obj, __u32 btf_id) __ksym __weak;

static __always_inline u64 get_task_cgroup_id(struct task_struct *task) {
    struct css_set *cgroups;
    u64 cgroup_id;
    bpf_rcu_read_lock();
    cgroups = task->cgroups;
    cgroup_id = cgroups->dfl_cgrp->kn->id;
    bpf_rcu_read_unlock();
    return cgroup_id;
}

static __always_inline u64 get_cgroup_pids_count(struct task_struct *task) {
    // ___local suffix + bpf_core_enum_value: CO-RE resolves the real pids_cgrp_id at load time
    enum cgroup_subsys_id___local {
        pids_cgrp_id___local = 123,
    };
    int cgrp_id = bpf_core_enum_value(enum cgroup_subsys_id___local, pids_cgrp_id___local);

    u64 count = 0;
    bpf_rcu_read_lock();
    struct cgroup_subsys_state *css = task->cgroups->subsys[cgrp_id];
    if (css) {
        struct pids_cgroup *pids = bpf_rdonly_cast(css, bpf_core_type_id_kernel(struct pids_cgroup));
        count = pids->counter.counter;
    }
    bpf_rcu_read_unlock();
    return count;
}

static __always_inline int enqueue_timestamp(struct task_struct *task) {
    u32 pid = task->pid;
    if (!pid) {
        return 0;
    }

    u64 ts = bpf_ktime_get_ns();
    u64 *ptr = bpf_task_storage_get(&runq_enqueued, task, &ts, BPF_LOCAL_STORAGE_GET_F_CREATE);
    if (!ptr) {
        return 0;
    }
    *ptr = ts;
    return 0;
}

static __always_inline cgroup_agg_stats_t *get_or_create_cgroup_stats(u64 cgroup_id) {
    cgroup_agg_stats_t *stats = bpf_map_lookup_elem(&cgroup_agg_stats, &cgroup_id);
    if (!stats) {
        cgroup_agg_stats_t zero = {};
        bpf_map_update_with_telemetry(cgroup_agg_stats, &cgroup_id, &zero, BPF_NOEXIST, -EEXIST);
        stats = bpf_map_lookup_elem(&cgroup_agg_stats, &cgroup_id);
    }
    return stats;
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
    // Fast gate: skip all detailed work if no cgroups are being watched
    u32 zero_key = 0;
    u8 *active = bpf_map_lookup_elem(&watchlist_active, &zero_key);
    if (!active || !*active) {
        // Still re-enqueue runnable tasks so timestamps are ready when watchlist activates
        struct task_struct *prev = (struct task_struct *)ctx[1];
        if (prev->__state == TASK_RUNNING) {
            enqueue_timestamp(prev);
        }
        return 0;
    }

    bool preempted = ctx[0] & 1;
    struct task_struct *prev = (struct task_struct *)ctx[1];
    struct task_struct *next = (struct task_struct *)ctx[2];
    u32 prev_pid = prev->pid;
    u32 next_pid = next->pid;

    if (prev->__state == TASK_RUNNING) {
        enqueue_timestamp(prev);
    }

    // Resolve cgroup IDs for both tasks
    u64 prev_cgroup_id = prev_pid ? get_task_cgroup_id(prev) : 0;
    u64 next_cgroup_id = next_pid ? get_task_cgroup_id(next) : 0;

    // Check if either cgroup is in the watchlist
    bool prev_watched = prev_pid && bpf_map_lookup_elem(&watchlist, &prev_cgroup_id);
    bool next_watched = next_pid && bpf_map_lookup_elem(&watchlist, &next_cgroup_id);

    if (!prev_watched && !next_watched) {
        return 0;
    }

    // Layer 2: Preemption classification for the victim (prev) cgroup
    if (preempted && prev_watched) {
        cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(prev_cgroup_id);
        if (stats) {
            if (prev_cgroup_id != next_cgroup_id) {
                stats->foreign_preemption_count += 1;

                // Layer 3: Track which foreign cgroup is doing the preempting
                preemptor_key_t pkey = {
                    .victim_cgroup_id = prev_cgroup_id,
                    .preemptor_cgroup_id = next_cgroup_id,
                };
                preemptor_stats_t *pstats = bpf_map_lookup_elem(&preemptor_stats, &pkey);
                if (pstats) {
                    pstats->count += 1;
                } else {
                    preemptor_stats_t new_pstats = { .count = 1 };
                    bpf_map_update_with_telemetry(preemptor_stats, &pkey, &new_pstats, BPF_NOEXIST, -EEXIST);
                }
            } else {
                stats->self_preemption_count += 1;
            }
        }
    }

    // Layer 2: Latency measurement for the scheduled-in (next) cgroup
    if (!next_watched || !next_pid) {
        return 0;
    }

    u64 *tsp = bpf_task_storage_get(&runq_enqueued, next, NULL, 0);
    if (!tsp) {
        return 0;
    }

    u64 runq_lat = bpf_ktime_get_ns() - *tsp;
    bpf_task_storage_delete(&runq_enqueued, next);

    cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(next_cgroup_id);
    if (stats) {
        stats->sum_latencies_ns += runq_lat;
        stats->event_count += 1;
        stats->task_count = get_cgroup_pids_count(next);

        // Latency histogram buckets
        if (runq_lat < 100000)
            stats->latency_bucket_lt_100us += 1;
        else if (runq_lat < 1000000)
            stats->latency_bucket_100us_1ms += 1;
        else if (runq_lat < 10000000)
            stats->latency_bucket_1ms_10ms += 1;
        else
            stats->latency_bucket_gt_10ms += 1;
    }

    return 0;
}

SEC("tp_btf/sched_migrate_task")
int tp_sched_migrate_task(u64 *ctx) {
    // Fast gate
    u32 zero_key = 0;
    u8 *active = bpf_map_lookup_elem(&watchlist_active, &zero_key);
    if (!active || !*active) {
        return 0;
    }

    struct task_struct *task = (struct task_struct *)ctx[0];
    u32 pid = task->pid;
    if (!pid) {
        return 0;
    }

    u64 cgroup_id = get_task_cgroup_id(task);
    if (!bpf_map_lookup_elem(&watchlist, &cgroup_id)) {
        return 0;
    }

    cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(cgroup_id);
    if (stats) {
        stats->cpu_migrations += 1;
    }

    return 0;
}

char _license[] SEC("license") = "GPL";
