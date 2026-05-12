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

BPF_PERCPU_HASH_MAP(cgroup_agg_stats, __u64, cgroup_agg_stats_t, MAX_TASK_ENTRIES)

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

// count_wakeup increments the per-cgroup wakeup counter for the given task.
// Called from the wakeup tracepoints only; the sched_switch re-enqueue path is
// a preemption-driven re-enqueue, not a wakeup, so it must not call this.
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

// Softirq accounting: stamp entry timestamp per CPU; on exit, attribute the
// elapsed nanoseconds to the cgroup of whatever task was running on this CPU
// when the softirq fired. For irq-tail softirq processing this charges back
// to the interrupted task's cgroup (the victim — what we want). For
// ksoftirqd-driven softirqs it charges to ksoftirqd (typically root); a
// known limitation.
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

// Block-I/O issue tracking: count requests issued from each cgroup, using the
// task that issued the I/O (current task) rather than the bio's owning cgroup.
// Simple and matches what cgroup CPU accounting attributes elsewhere.
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
