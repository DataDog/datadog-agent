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
    long cyc_err = bpf_perf_event_read_value(&cycles_pmu, BPF_F_CURRENT_CPU, &cyc_val, sizeof(cyc_val));
    long ins_err = bpf_perf_event_read_value(&instructions_pmu, BPF_F_CURRENT_CPU, &ins_val, sizeof(ins_val));
    long llc_err = bpf_perf_event_read_value(&llc_misses_pmu, BPF_F_CURRENT_CPU, &llc_val, sizeof(llc_val));
    long itlb_err = bpf_perf_event_read_value(&itlb_misses_pmu, BPF_F_CURRENT_CPU, &itlb_val, sizeof(itlb_val));
    bool ci_ok = (cyc_err == 0) && (ins_err == 0);
    bool llc_ok = (llc_err == 0);
    bool itlb_ok = (itlb_err == 0);
    bool pmu_ok = ci_ok || llc_ok || itlb_ok;

    if (pmu_ok && prev_pid) {
        task_pmu_stamp_t *stamp = bpf_task_storage_get(&task_oncpu_pmu, prev, NULL, 0);
        if (stamp) {
            u64 prev_cgroup_id = get_task_cgroup_id(prev);
            cgroup_agg_stats_t *stats = get_or_create_cgroup_stats(prev_cgroup_id);
            if (stats) {
                if (ci_ok) {
                    stats->sum_cycles += cyc_val.counter - stamp->cycles;
                    stats->sum_instructions += ins_val.counter - stamp->instructions;
                }
                if (llc_ok) {
                    stats->sum_llc_misses += llc_val.counter - stamp->llc_misses;
                }
                if (itlb_ok) {
                    stats->sum_itlb_misses += itlb_val.counter - stamp->itlb_misses;
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
                stamp->cycles = cyc_val.counter;
                stamp->instructions = ins_val.counter;
            }
            if (llc_ok) {
                stamp->llc_misses = llc_val.counter;
            }
            if (itlb_ok) {
                stamp->itlb_misses = itlb_val.counter;
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
