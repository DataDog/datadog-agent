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

BPF_TASK_STORAGE_MAP(runq_enqueued, u64)

BPF_TASK_STORAGE_MAP(task_oncpu_pmu, task_pmu_stamp_t)

BPF_PERCPU_HASH_MAP(cgroup_agg_stats, __u64, cgroup_agg_stats_t, MAX_TASK_ENTRIES)

// Per-CPU perf event arrays for hardware counters. Populated from user space
// at probe init; if a CPU's slot is unset, the read helper returns -ENOENT
// and the corresponding deltas are skipped.
BPF_PERF_EVENT_ARRAY_MAP(cycles_pmu, u32)
BPF_PERF_EVENT_ARRAY_MAP(instructions_pmu, u32)
BPF_PERF_EVENT_ARRAY_MAP(cache_misses_pmu, u32)
BPF_PERF_EVENT_ARRAY_MAP(itlb_misses_pmu, u32)
BPF_PERF_EVENT_ARRAY_MAP(branch_misses_pmu, u32)
BPF_PERF_EVENT_ARRAY_MAP(cpu_migrations_pmu, u32)
BPF_PERF_EVENT_ARRAY_MAP(cache_references_pmu, u32)

// Per-CPU softirq entry timestamp. Single-slot percpu array; softirq runs in
// non-preemptible context so per-CPU storage is sufficient.
BPF_PERCPU_ARRAY_MAP(softirq_start_ns, __u64, 1)

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

// scaled_pmu_delta computes (val.counter - stamp.counter) scaled by
// enabled/running to undo any time-multiplexing the kernel applied to the
// underlying hardware counter. When enabled == running (no multiplexing),
// the result equals the raw delta. When the event was paused for the
// entire window (running_delta == 0), returns 0 — the sample carries no
// usable information.
static __always_inline u64 scaled_pmu_delta(struct bpf_perf_event_value *val, pmu_event_stamp_t *stamp) {
    u64 raw = val->counter - stamp->counter;
    u64 enabled_delta = val->enabled - stamp->enabled;
    u64 running_delta = val->running - stamp->running;
    if (running_delta == 0) {
        return 0;
    }
    if (enabled_delta == running_delta) {
        return raw;
    }
    // Compute as raw + raw*(enabled-running)/running rather than
    // raw*enabled/running to keep the multiplication operands small in
    // the common multiplexing case (enabled-running << running). This
    // reduces — but does not eliminate — u64 overflow risk on very long
    // sched windows.
    u64 missing = enabled_delta - running_delta;
    return raw + (raw * missing) / running_delta;
}

// pmu_sample_t holds one sample of every PMU event we track, along with
// per-event ok flags marking which reads succeeded. A failed read leaves
// the bpf_perf_event_value buffer at zero; the ok flag prevents that zero
// from being used as a stamp baseline (which would produce a huge bogus
// delta on the next sched_switch) or as an accumulator input.
//
// Every counter is gated independently: cycles can be on while instructions
// is off, etc. (CPI, if needed, is computed dashboard-side from the raw
// counters.) When a userspace toggle disables a perf event, that event's
// perf-event-array map stays empty, the read returns -ENOENT, the ok flag
// stays false, and accumulation skips that counter only.
typedef struct {
    struct bpf_perf_event_value cycles;
    struct bpf_perf_event_value instructions;
    struct bpf_perf_event_value cache_misses;
    struct bpf_perf_event_value itlb_misses;
    struct bpf_perf_event_value branch_misses;
    struct bpf_perf_event_value cpu_migrations;
    struct bpf_perf_event_value cache_references;
    bool cycles_ok;
    bool instructions_ok;
    bool cache_misses_ok;
    bool itlb_ok;
    bool branch_misses_ok;
    bool cpu_migrations_ok;
    bool cache_references_ok;
} pmu_sample_t;

static __always_inline void pmu_sample_all(pmu_sample_t *s) {
    s->cycles_ok = bpf_perf_event_read_value(&cycles_pmu, BPF_F_CURRENT_CPU, &s->cycles, sizeof(s->cycles)) == 0;
    s->instructions_ok = bpf_perf_event_read_value(&instructions_pmu, BPF_F_CURRENT_CPU, &s->instructions, sizeof(s->instructions)) == 0;
    s->cache_misses_ok = bpf_perf_event_read_value(&cache_misses_pmu, BPF_F_CURRENT_CPU, &s->cache_misses, sizeof(s->cache_misses)) == 0;
    s->itlb_ok = bpf_perf_event_read_value(&itlb_misses_pmu, BPF_F_CURRENT_CPU, &s->itlb_misses, sizeof(s->itlb_misses)) == 0;
    s->branch_misses_ok = bpf_perf_event_read_value(&branch_misses_pmu, BPF_F_CURRENT_CPU, &s->branch_misses, sizeof(s->branch_misses)) == 0;
    s->cpu_migrations_ok = bpf_perf_event_read_value(&cpu_migrations_pmu, BPF_F_CURRENT_CPU, &s->cpu_migrations, sizeof(s->cpu_migrations)) == 0;
    s->cache_references_ok = bpf_perf_event_read_value(&cache_references_pmu, BPF_F_CURRENT_CPU, &s->cache_references, sizeof(s->cache_references)) == 0;
}

static __always_inline bool pmu_any_ok(pmu_sample_t *s) {
    return s->cycles_ok || s->instructions_ok || s->cache_misses_ok || s->itlb_ok ||
           s->branch_misses_ok || s->cpu_migrations_ok || s->cache_references_ok;
}

static __always_inline void pmu_stamp_event(pmu_event_stamp_t *slot, struct bpf_perf_event_value *val) {
    slot->counter = val->counter;
    slot->enabled = val->enabled;
    slot->running = val->running;
}

static __always_inline void pmu_stamp_from_sample(task_pmu_stamp_t *stamp, pmu_sample_t *s) {
    if (s->cycles_ok) {
        pmu_stamp_event(&stamp->cycles, &s->cycles);
    }
    if (s->instructions_ok) {
        pmu_stamp_event(&stamp->instructions, &s->instructions);
    }
    if (s->cache_misses_ok) {
        pmu_stamp_event(&stamp->cache_misses, &s->cache_misses);
    }
    if (s->itlb_ok) {
        pmu_stamp_event(&stamp->itlb_misses, &s->itlb_misses);
    }
    if (s->branch_misses_ok) {
        pmu_stamp_event(&stamp->branch_misses, &s->branch_misses);
    }
    if (s->cpu_migrations_ok) {
        pmu_stamp_event(&stamp->cpu_migrations, &s->cpu_migrations);
    }
    if (s->cache_references_ok) {
        pmu_stamp_event(&stamp->cache_references, &s->cache_references);
    }
}

static __always_inline void pmu_accum_to_stats(cgroup_agg_stats_t *stats, task_pmu_stamp_t *stamp, pmu_sample_t *s) {
    if (s->cycles_ok) {
        stats->sum_cycles += scaled_pmu_delta(&s->cycles, &stamp->cycles);
    }
    if (s->instructions_ok) {
        stats->sum_instructions += scaled_pmu_delta(&s->instructions, &stamp->instructions);
    }
    if (s->cache_misses_ok) {
        stats->sum_cache_misses += scaled_pmu_delta(&s->cache_misses, &stamp->cache_misses);
    }
    if (s->itlb_ok) {
        stats->sum_itlb_misses += scaled_pmu_delta(&s->itlb_misses, &stamp->itlb_misses);
    }
    if (s->branch_misses_ok) {
        stats->sum_branch_misses += scaled_pmu_delta(&s->branch_misses, &stamp->branch_misses);
    }
    if (s->cpu_migrations_ok) {
        stats->sum_cpu_migrations += scaled_pmu_delta(&s->cpu_migrations, &stamp->cpu_migrations);
    }
    if (s->cache_references_ok) {
        stats->sum_cache_references += scaled_pmu_delta(&s->cache_references, &stamp->cache_references);
    }
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

// task_cgroup_stats returns the cgroup aggregate stats entry for the given
// task, creating it on first observation. Returns NULL if the map insert fails.
static __always_inline cgroup_agg_stats_t *task_cgroup_stats(struct task_struct *task) {
    u64 cgroup_id = get_task_cgroup_id(task);
    return get_or_create_cgroup_stats(cgroup_id);
}

// current_cgroup_stats returns the cgroup aggregate stats entry for the
// currently-running task. Returns NULL if the current task pointer can't be
// resolved or if the map insert fails.
static __always_inline cgroup_agg_stats_t *current_cgroup_stats(void) {
    struct task_struct *task = bpf_get_current_task_btf();
    if (!task) {
        return NULL;
    }
    return task_cgroup_stats(task);
}

// count_wakeup increments the per-cgroup wakeup counter for the given task.
// Called only from the genuine wakeup tracepoints — not from the sched_switch
// re-enqueue path (which is a preemption-driven re-enqueue, not a wakeup).
static __always_inline void count_wakeup(struct task_struct *task) {
    cgroup_agg_stats_t *stats = task_cgroup_stats(task);
    if (stats) {
        stats->wakeup_count += 1;
    }
}

// handle_wakeup is the shared body of tp_sched_wakeup and tp_sched_wakeup_new.
// Both tracepoints carry the same (task) shape and need the same accounting:
// count the wakeup, then stamp the enqueue timestamp.
static __always_inline int handle_wakeup(struct task_struct *task) {
    count_wakeup(task);
    return enqueue_timestamp(task);
}

SEC("tp_btf/sched_wakeup")
int tp_sched_wakeup(u64 *ctx) {
    return handle_wakeup((struct task_struct *)ctx[0]);
}

SEC("tp_btf/sched_wakeup_new")
int tp_sched_wakeup_new(u64 *ctx) {
    return handle_wakeup((struct task_struct *)ctx[0]);
}

SEC("tp_btf/sched_switch")
int tp_sched_switch(u64 *ctx) {
    bool preempted = ctx[0] & 1;
    struct task_struct *prev = (struct task_struct *)ctx[1];
    struct task_struct *next = (struct task_struct *)ctx[2];
    u32 prev_pid = prev->pid;
    u32 next_pid = next->pid;

    // Sample PMU counters once. Each read is independent — if iTLB events
    // aren't supported but cycles are, only iTLB deltas are skipped.
    pmu_sample_t pmu = {};
    pmu_sample_all(&pmu);
    bool pmu_ok = pmu_any_ok(&pmu);

    if (pmu_ok && prev_pid) {
        task_pmu_stamp_t *stamp = bpf_task_storage_get(&task_oncpu_pmu, prev, NULL, 0);
        if (stamp) {
            cgroup_agg_stats_t *stats = task_cgroup_stats(prev);
            if (stats) {
                pmu_accum_to_stats(stats, stamp, &pmu);
            }
        }
    }

    if (prev->__state == TASK_RUNNING) {
        enqueue_timestamp(prev);
    }

    if (preempted && prev_pid) {
        cgroup_agg_stats_t *stats = task_cgroup_stats(prev);
        if (stats) {
            stats->preemption_count += 1;
        }
    }

    if (!next_pid) {
        return 0;
    }

    if (pmu_ok) {
        task_pmu_stamp_t zero = {};
        task_pmu_stamp_t *stamp = bpf_task_storage_get(&task_oncpu_pmu, next, &zero, BPF_LOCAL_STORAGE_GET_F_CREATE);
        if (stamp) {
            pmu_stamp_from_sample(stamp, &pmu);
        }
    }

    u64 *tsp = bpf_task_storage_get(&runq_enqueued, next, NULL, 0);
    if (!tsp) {
        return 0;
    }

    u64 runq_lat = bpf_ktime_get_ns() - *tsp;
    bpf_task_storage_delete(&runq_enqueued, next);

    cgroup_agg_stats_t *stats = task_cgroup_stats(next);
    if (stats) {
        stats->sum_latencies_ns += runq_lat;
        stats->event_count += 1;
        stats->pid_count = get_cgroup_pids_count(next);
    }

    return 0;
}

// Softirq accounting: stamp the entry timestamp per CPU; on exit, attribute
// the elapsed nanoseconds to the cgroup of whatever task was running on this
// CPU when the softirq fired. For irq-tail softirq processing, this charges
// time back to the interrupted task's cgroup (the victim — exactly what we
// want). For ksoftirqd-driven softirqs, it charges to the ksoftirqd cgroup
// (typically root); a known limitation we accept for the prototype.
SEC("tp_btf/softirq_entry")
int tp_softirq_entry(u64 *ctx) {
    u32 zero = 0;
    u64 *slot = bpf_map_lookup_elem(&softirq_start_ns, &zero);
    if (slot) {
        *slot = bpf_ktime_get_ns();
    }
    return 0;
}

SEC("tp_btf/softirq_exit")
int tp_softirq_exit(u64 *ctx) {
    u32 zero = 0;
    u64 *slot = bpf_map_lookup_elem(&softirq_start_ns, &zero);
    if (!slot || *slot == 0) {
        return 0;
    }
    u64 delta = bpf_ktime_get_ns() - *slot;
    *slot = 0;

    cgroup_agg_stats_t *stats = current_cgroup_stats();
    if (stats) {
        stats->sum_softirq_ns += delta;
    }
    return 0;
}

// Block I/O issue tracking: count requests issued from each cgroup. Uses the
// task that issued the I/O (current task), not the bio's owning cgroup —
// simple and matches what cgroup CPU accounting attributes elsewhere.
SEC("tp_btf/block_rq_issue")
int tp_block_rq_issue(u64 *ctx) {
    cgroup_agg_stats_t *stats = current_cgroup_stats();
    if (stats) {
        stats->block_io_requests += 1;
    }
    return 0;
}

char _license[] SEC("license") = "GPL";
