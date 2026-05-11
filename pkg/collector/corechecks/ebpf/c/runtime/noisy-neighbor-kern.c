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
BPF_PERF_EVENT_ARRAY_MAP(llc_misses_pmu, u32)
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

static __always_inline cgroup_agg_stats_t *get_or_create_cgroup_stats(u64 cgroup_id) {
    cgroup_agg_stats_t *stats = bpf_map_lookup_elem(&cgroup_agg_stats, &cgroup_id);
    if (!stats) {
        cgroup_agg_stats_t zero = {};
        bpf_map_update_with_telemetry(cgroup_agg_stats, &cgroup_id, &zero, BPF_NOEXIST, -EEXIST);
        stats = bpf_map_lookup_elem(&cgroup_agg_stats, &cgroup_id);
    }
    return stats;
}

// count_wakeup increments the per-cgroup wakeup counter for the given task.
// Called only from the genuine wakeup tracepoints — not from the sched_switch
// re-enqueue path (which is a preemption-driven re-enqueue, not a wakeup).
static __always_inline void count_wakeup(struct task_struct *task) {
    u64 cgroup_id = get_task_cgroup_id(task);
    cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(cgroup_id);
    if (stats) {
        stats->wakeup_count += 1;
    }
}

SEC("tp_btf/sched_wakeup")
int tp_sched_wakeup(u64 *ctx) {
    struct task_struct *task = (void *)ctx[0];
    count_wakeup(task);
    return enqueue_timestamp(task);
}

SEC("tp_btf/sched_wakeup_new")
int tp_sched_wakeup_new(u64 *ctx) {
    struct task_struct *task = (void *)ctx[0];
    count_wakeup(task);
    return enqueue_timestamp(task);
}

SEC("tp_btf/sched_switch")
int tp_sched_switch(u64 *ctx) {
    bool preempted = ctx[0] & 1;
    struct task_struct *prev = (struct task_struct *)ctx[1];
    struct task_struct *next = (struct task_struct *)ctx[2];
    u32 prev_pid = prev->pid;
    u32 next_pid = next->pid;

    // Sample PMU counters once. Reads return -ENOENT (or other negative) when
    // perf events haven't been attached for this CPU; in that case we skip
    // both the prev close-out and the next stamp. The runqueue-wait path
    // below is unaffected. Each counter is independent — if iTLB events
    // aren't supported but cycles are, only iTLB deltas are skipped.
    struct bpf_perf_event_value cyc_val = {};
    struct bpf_perf_event_value ins_val = {};
    struct bpf_perf_event_value llc_val = {};
    struct bpf_perf_event_value itlb_val = {};
    struct bpf_perf_event_value bm_val = {};
    struct bpf_perf_event_value cm_val = {};
    struct bpf_perf_event_value cr_val = {};
    long cyc_err = bpf_perf_event_read_value(&cycles_pmu, BPF_F_CURRENT_CPU, &cyc_val, sizeof(cyc_val));
    long ins_err = bpf_perf_event_read_value(&instructions_pmu, BPF_F_CURRENT_CPU, &ins_val, sizeof(ins_val));
    long llc_err = bpf_perf_event_read_value(&llc_misses_pmu, BPF_F_CURRENT_CPU, &llc_val, sizeof(llc_val));
    long itlb_err = bpf_perf_event_read_value(&itlb_misses_pmu, BPF_F_CURRENT_CPU, &itlb_val, sizeof(itlb_val));
    long bm_err = bpf_perf_event_read_value(&branch_misses_pmu, BPF_F_CURRENT_CPU, &bm_val, sizeof(bm_val));
    long cm_err = bpf_perf_event_read_value(&cpu_migrations_pmu, BPF_F_CURRENT_CPU, &cm_val, sizeof(cm_val));
    long cr_err = bpf_perf_event_read_value(&cache_references_pmu, BPF_F_CURRENT_CPU, &cr_val, sizeof(cr_val));
    bool ci_ok = (cyc_err == 0) && (ins_err == 0);
    bool llc_ok = (llc_err == 0);
    bool itlb_ok = (itlb_err == 0);
    bool bm_ok = (bm_err == 0);
    bool cm_ok = (cm_err == 0);
    bool cr_ok = (cr_err == 0);
    bool pmu_ok = ci_ok || llc_ok || itlb_ok || bm_ok || cm_ok || cr_ok;

    if (pmu_ok && prev_pid) {
        task_pmu_stamp_t *stamp = bpf_task_storage_get(&task_oncpu_pmu, prev, NULL, 0);
        if (stamp) {
            u64 prev_cgroup_id = get_task_cgroup_id(prev);
            cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(prev_cgroup_id);
            if (stats) {
                if (ci_ok) {
                    stats->sum_cycles += scaled_pmu_delta(&cyc_val, &stamp->cycles);
                    stats->sum_instructions += scaled_pmu_delta(&ins_val, &stamp->instructions);
                }
                if (llc_ok) {
                    stats->sum_llc_misses += scaled_pmu_delta(&llc_val, &stamp->llc_misses);
                }
                if (itlb_ok) {
                    stats->sum_itlb_misses += scaled_pmu_delta(&itlb_val, &stamp->itlb_misses);
                }
                if (bm_ok) {
                    stats->sum_branch_misses += scaled_pmu_delta(&bm_val, &stamp->branch_misses);
                }
                if (cm_ok) {
                    stats->sum_cpu_migrations += scaled_pmu_delta(&cm_val, &stamp->cpu_migrations);
                }
                if (cr_ok) {
                    stats->sum_cache_references += scaled_pmu_delta(&cr_val, &stamp->cache_references);
                }
            }
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

    if (pmu_ok) {
        task_pmu_stamp_t zero = {};
        task_pmu_stamp_t *stamp = bpf_task_storage_get(&task_oncpu_pmu, next, &zero, BPF_LOCAL_STORAGE_GET_F_CREATE);
        if (stamp) {
            if (ci_ok) {
                stamp->cycles.counter = cyc_val.counter;
                stamp->cycles.enabled = cyc_val.enabled;
                stamp->cycles.running = cyc_val.running;
                stamp->instructions.counter = ins_val.counter;
                stamp->instructions.enabled = ins_val.enabled;
                stamp->instructions.running = ins_val.running;
            }
            if (llc_ok) {
                stamp->llc_misses.counter = llc_val.counter;
                stamp->llc_misses.enabled = llc_val.enabled;
                stamp->llc_misses.running = llc_val.running;
            }
            if (itlb_ok) {
                stamp->itlb_misses.counter = itlb_val.counter;
                stamp->itlb_misses.enabled = itlb_val.enabled;
                stamp->itlb_misses.running = itlb_val.running;
            }
            if (bm_ok) {
                stamp->branch_misses.counter = bm_val.counter;
                stamp->branch_misses.enabled = bm_val.enabled;
                stamp->branch_misses.running = bm_val.running;
            }
            if (cm_ok) {
                stamp->cpu_migrations.counter = cm_val.counter;
                stamp->cpu_migrations.enabled = cm_val.enabled;
                stamp->cpu_migrations.running = cm_val.running;
            }
            if (cr_ok) {
                stamp->cache_references.counter = cr_val.counter;
                stamp->cache_references.enabled = cr_val.enabled;
                stamp->cache_references.running = cr_val.running;
            }
        }
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

    struct task_struct *task = bpf_get_current_task_btf();
    if (!task) {
        return 0;
    }
    u64 cgroup_id = get_task_cgroup_id(task);
    cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(cgroup_id);
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
    struct task_struct *task = bpf_get_current_task_btf();
    if (!task) {
        return 0;
    }
    u64 cgroup_id = get_task_cgroup_id(task);
    cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(cgroup_id);
    if (stats) {
        stats->block_io_requests += 1;
    }
    return 0;
}

char _license[] SEC("license") = "GPL";
