#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "noisy-neighbor-kern-user.h"
#include "bpf_metadata.h"
#include "bpf_telemetry.h"

#define MAX_TASK_ENTRIES 4096
#define MAX_WATCHED_CGROUPS 128
#define TASK_RUNNING 0

#define EVENT_CYCLES (1ULL << 0)
#define EVENT_INSTRUCTIONS (1ULL << 1)
#define EVENT_CACHE_MISSES (1ULL << 2)
#define EVENT_CACHE_REFERENCES (1ULL << 3)
#define EVENT_ITLB_MISSES (1ULL << 4)
#define EVENT_BRANCH_MISSES (1ULL << 5)
#define EVENT_CPU_MIGRATIONS (1ULL << 6)
#define HARDWARE_EVENT_MASK (EVENT_CYCLES | EVENT_INSTRUCTIONS | EVENT_CACHE_MISSES | EVENT_CACHE_REFERENCES | EVENT_ITLB_MISSES | EVENT_BRANCH_MISSES)

volatile const __u64 pmu_event_mask = 0;

typedef struct {
    __u64 cgroup_id;
    __u64 generation;
    __u64 valid_event_mask;
    pmu_counter_t counters[NOISY_NEIGHBOR_HARDWARE_EVENTS];
} pmu_task_state_t;

BPF_TASK_STORAGE_MAP(runq_enqueued, u64)
BPF_TASK_STORAGE_MAP(pmu_task_state, pmu_task_state_t)

BPF_PERCPU_HASH_MAP(cgroup_agg_stats, __u64, cgroup_agg_stats_t, MAX_TASK_ENTRIES)
BPF_PERCPU_HASH_MAP(pmu_cgroup_stats, __u64, pmu_cgroup_stats_t, MAX_WATCHED_CGROUPS)
BPF_HASH_MAP(pmu_watchlist, __u64, __u64, MAX_WATCHED_CGROUPS)
BPF_ARRAY_MAP(pmu_config, pmu_config_t, 1)
BPF_PERCPU_ARRAY_MAP(pmu_error_stats, pmu_error_stats_t, 1)
BPF_PERF_EVENT_ARRAY_MAP(pmu_cycles, __u32)
BPF_PERF_EVENT_ARRAY_MAP(pmu_instructions, __u32)
BPF_PERF_EVENT_ARRAY_MAP(pmu_cache_misses, __u32)
BPF_PERF_EVENT_ARRAY_MAP(pmu_cache_references, __u32)
BPF_PERF_EVENT_ARRAY_MAP(pmu_itlb_misses, __u32)
BPF_PERF_EVENT_ARRAY_MAP(pmu_branch_misses, __u32)

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

static __always_inline pmu_cgroup_stats_t *get_or_create_pmu_stats(u64 cgroup_id) {
    pmu_cgroup_stats_t *stats = bpf_map_lookup_elem(&pmu_cgroup_stats, &cgroup_id);
    if (!stats) {
        pmu_cgroup_stats_t zero = {};
        bpf_map_update_with_telemetry(pmu_cgroup_stats, &cgroup_id, &zero, BPF_NOEXIST, -EEXIST);
        stats = bpf_map_lookup_elem(&pmu_cgroup_stats, &cgroup_id);
    }
    return stats;
}

static __always_inline bool read_one_event(void *map, pmu_counter_t *counter) {
    struct bpf_perf_event_value value = {};
    if (bpf_perf_event_read_value(map, BPF_F_CURRENT_CPU, &value, sizeof(value)) != 0) {
        __u32 key = 0;
        pmu_error_stats_t *errors = bpf_map_lookup_elem(&pmu_error_stats, &key);
        if (errors)
            errors->read_errors++;
        return false;
    }
    counter->value = value.counter;
    counter->enabled = value.enabled;
    counter->running = value.running;
    return true;
}

static __always_inline __u64 read_pmu_events(pmu_counter_t *counters, __u64 mask) {
    __u64 valid = 0;
    if ((mask & EVENT_CYCLES) && read_one_event(&pmu_cycles, &counters[0]))
        valid |= EVENT_CYCLES;
    if ((mask & EVENT_INSTRUCTIONS) && read_one_event(&pmu_instructions, &counters[1]))
        valid |= EVENT_INSTRUCTIONS;
    if ((mask & EVENT_CACHE_MISSES) && read_one_event(&pmu_cache_misses, &counters[2]))
        valid |= EVENT_CACHE_MISSES;
    if ((mask & EVENT_CACHE_REFERENCES) && read_one_event(&pmu_cache_references, &counters[3]))
        valid |= EVENT_CACHE_REFERENCES;
    if ((mask & EVENT_ITLB_MISSES) && read_one_event(&pmu_itlb_misses, &counters[4]))
        valid |= EVENT_ITLB_MISSES;
    if ((mask & EVENT_BRANCH_MISSES) && read_one_event(&pmu_branch_misses, &counters[5]))
        valid |= EVENT_BRANCH_MISSES;
    return valid;
}

static __always_inline void account_pmu_switch_out(struct task_struct *task, pmu_config_t *config) {
    pmu_task_state_t *baseline = bpf_task_storage_get(&pmu_task_state, task, NULL, 0);
    if (!baseline)
        return;

    __u64 cgroup_id = baseline->cgroup_id;
    __u64 *generation = bpf_map_lookup_elem(&pmu_watchlist, &cgroup_id);
    if (!generation || baseline->generation != *generation) {
        bpf_task_storage_delete(&pmu_task_state, task);
        return;
    }

    pmu_counter_t current[NOISY_NEIGHBOR_HARDWARE_EVENTS] = {};
    __u64 valid = read_pmu_events(current, config->effective_event_mask) & baseline->valid_event_mask;
    pmu_cgroup_stats_t *stats = get_or_create_pmu_stats(baseline->cgroup_id);
    if (!stats) {
        bpf_task_storage_delete(&pmu_task_state, task);
        return;
    }

#pragma unroll
    for (int i = 0; i < NOISY_NEIGHBOR_HARDWARE_EVENTS; i++) {
        __u64 bit = 1ULL << i;
        if (!(valid & bit))
            continue;
        if (current[i].value < baseline->counters[i].value ||
            current[i].enabled < baseline->counters[i].enabled ||
            current[i].running < baseline->counters[i].running)
            continue;
        stats->counters[i].value += current[i].value - baseline->counters[i].value;
        stats->counters[i].enabled += current[i].enabled - baseline->counters[i].enabled;
        stats->counters[i].running += current[i].running - baseline->counters[i].running;
        stats->sampled_event_mask |= bit;
    }
    bpf_task_storage_delete(&pmu_task_state, task);
}

static __always_inline void record_pmu_switch_in(struct task_struct *task, pmu_config_t *config) {
    __u64 cgroup_id = get_task_cgroup_id(task);
    __u64 *generation = bpf_map_lookup_elem(&pmu_watchlist, &cgroup_id);
    if (!generation)
        return;

    pmu_task_state_t zero = {};
    pmu_task_state_t *state = bpf_task_storage_get(&pmu_task_state, task, &zero, BPF_LOCAL_STORAGE_GET_F_CREATE);
    if (!state)
        return;
    state->cgroup_id = cgroup_id;
    state->generation = *generation;
    state->valid_event_mask = read_pmu_events(state->counters, config->effective_event_mask);
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
    bool preempted = ctx[0] & 1;
    struct task_struct *prev = (struct task_struct *)ctx[1];
    struct task_struct *next = (struct task_struct *)ctx[2];
    u32 prev_pid = prev->pid;
    u32 next_pid = next->pid;

    if (pmu_event_mask & HARDWARE_EVENT_MASK) {
        __u32 key = 0;
        pmu_config_t *config = bpf_map_lookup_elem(&pmu_config, &key);
        if (config && config->active && (config->effective_event_mask & HARDWARE_EVENT_MASK)) {
            if (prev_pid)
                account_pmu_switch_out(prev, config);
            if (next_pid)
                record_pmu_switch_in(next, config);
        }
    }

    if (prev->__state == TASK_RUNNING) {
        enqueue_timestamp(prev);
    }

    if (preempted && prev_pid) {
        u64 prev_cgroup_id = get_task_cgroup_id(prev);
        cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(prev_cgroup_id);
        if (stats) {
            stats->preemption_count += 1;
        }
    }

    if (!next_pid) {
        return 0;
    }

    u64 *tsp = bpf_task_storage_get(&runq_enqueued, next, NULL, 0);
    if (!tsp) {
        return 0;
    }

    u64 runq_lat = bpf_ktime_get_ns() - *tsp;
    bpf_task_storage_delete(&runq_enqueued, next);

    u64 cgroup_id = get_task_cgroup_id(next);
    cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(cgroup_id);
    if (stats) {
        stats->sum_latencies_ns += runq_lat;
        stats->event_count += 1;
        stats->pid_count = get_cgroup_pids_count(next);
    }

    return 0;
}

SEC("tp_btf/sched_migrate_task")
int tp_sched_migrate_task(u64 *ctx) {
    if (!(pmu_event_mask & EVENT_CPU_MIGRATIONS))
        return 0;

    __u32 key = 0;
    pmu_config_t *config = bpf_map_lookup_elem(&pmu_config, &key);
    if (!config || !config->active || !(config->effective_event_mask & EVENT_CPU_MIGRATIONS))
        return 0;

    struct task_struct *task = (struct task_struct *)ctx[0];
    __u64 cgroup_id = get_task_cgroup_id(task);
    __u64 *generation = bpf_map_lookup_elem(&pmu_watchlist, &cgroup_id);
    if (!generation)
        return 0;

    pmu_cgroup_stats_t *stats = get_or_create_pmu_stats(cgroup_id);
    if (stats) {
        stats->cpu_migrations += 1;
        stats->sampled_event_mask |= EVENT_CPU_MIGRATIONS;
    }
    return 0;
}

char _license[] SEC("license") = "GPL";
