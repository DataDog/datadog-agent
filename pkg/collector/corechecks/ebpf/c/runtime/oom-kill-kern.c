#include "ktypes.h"
#include "bpf_metadata.h"

#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#include <linux/oom.h>
#include <linux/memcontrol.h>

#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 9, 0)
// 4.8 is the first version where `struct oom_control*` is the first argument of `oom_kill_process`
// 4.9 is the first version where the field `totalpages` is available in `struct oom_control`
#error Versions of Linux previous to 4.9.0 are not supported by this probe
#endif

#endif

#include "oom-kill-kern-user.h"
#include "cgroup.h"

#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "compiler.h"

/*
 * The `oom_stats` hash map is used to share with the userland program system-probe
 * the statistics per pid
 */

BPF_HASH_MAP(oom_stats, struct stats_key, struct oom_stats, 10240)

// The object must not declare __kconfig externs: with a .kconfig section, cilium treats the
// LOAD_CONSTANT references below as kconfig reads and fails to load the object.

// emit_cgroup_memory_stats is set by system-probe from oom_kill.emit_cgroup_memory_stats
static __always_inline bool emit_cgroup_memory_stats() {
    __u64 val = 0;
    LOAD_CONSTANT("emit_cgroup_memory_stats", val);
    return val == 1;
}

#ifdef COMPILE_CORE
#ifndef PAGE_SIZE
#define PAGE_SIZE 4096
#endif

#ifndef READ_ONCE
#define READ_ONCE(x) (*(const volatile typeof(x) *)&(x))
#endif

// Since 6.10 (ff48c71c26aa) memcg_vmstats only holds the stat items tracked by memcg, and the kernel
// maps a stat item to its slot through this table, which is not in BTF.
extern const void mem_cgroup_stats_index __ksym __weak;
// Used to locate the per-cpu copies of memcg->vmstats_percpu, like per_cpu_ptr() does.
extern const void __per_cpu_offset __ksym __weak;
extern const void nr_cpu_ids __ksym __weak;

/*
 * `memcg_flush_seq` is a sequence count of the memcg stats flush steps: it is odd while
 * mem_cgroup_css_rstat_flush runs, and changes with every call. The calls are serialized by the
 * memcg rstat lock, so a single counter is enough. The stat items are only reported once read
 * without a flush step overlapping, which could count the updates it moves twice or miss them.
 */
BPF_ARRAY_MAP(memcg_flush_seq, __u64, 1)

static __always_inline __u64 *memcg_flush_seq_ptr() {
    u32 key = 0;
    return bpf_map_lookup_elem(&memcg_flush_seq, &key);
}

// The writers use atomics, which are fully ordered, so that the counter changes are not reordered
// with the writes of the flush step. The reader cannot: kernels before 5.12 reject atomics with
// BPF_FETCH anywhere in a program, even in dead code, and the oom-kill programs load on them. It
// uses plain loads.

SEC("fentry/mem_cgroup_css_rstat_flush")
int BPF_PROG(fentry__mem_cgroup_css_rstat_flush) {
    __u64 *seq = memcg_flush_seq_ptr();
    if (!seq)
        return 0;

    // only from even, in case the program was attached while a flush step was running
    __u64 cur = *seq;
    if (!(cur & 1))
        __sync_val_compare_and_swap(seq, cur, cur + 1);
    return 0;
}

SEC("fexit/mem_cgroup_css_rstat_flush")
int BPF_PROG(fexit__mem_cgroup_css_rstat_flush) {
    __u64 *seq = memcg_flush_seq_ptr();
    if (!seq)
        return 0;

    __u64 cur = *seq;
    if (cur & 1)
        __sync_val_compare_and_swap(seq, cur, cur + 1);
    return 0;
}

// Number of attempts at reading the stat items without overlapping a flush step. It is unrolled,
// rather than looped, so that the program still loads on kernels without bounded loops.
#define CGROUP_MEM_STATS_ATTEMPTS 3

enum cgroup_memory_stat {
    CGROUP_MEM_ACTIVE_ANON,
    CGROUP_MEM_INACTIVE_ANON,
    CGROUP_MEM_ACTIVE_FILE,
    CGROUP_MEM_INACTIVE_FILE,
    CGROUP_MEM_UNEVICTABLE,
    CGROUP_MEM_KERNEL,
    CGROUP_MEM_NR_STATS,
};

struct cgroup_memory_stats_ctx {
    struct memcg_vmstats_percpu *pcpu;
    long slot[CGROUP_MEM_NR_STATS];
    long pages[CGROUP_MEM_NR_STATS];
};

// mem_cgroup_stats_slot returns the slot of the stat item idx in the memcg vmstats state arrays, or
// -1 if memcg does not track it.
static __always_inline long mem_cgroup_stats_slot(int idx) {
    if (idx < 0 || idx >= bpf_core_enum_value(enum memcg_stat_item, MEMCG_NR_STAT))
        return -1;

    // before 6.10 the state arrays hold every stat item and are indexed by the item itself
    if (bpf_core_field_size(struct memcg_vmstats, state) >= bpf_core_enum_value(enum memcg_stat_item, MEMCG_NR_STAT) * sizeof(long))
        return idx;

    if (!&mem_cgroup_stats_index)
        return -1;

    u8 slot = 0xff;
    bpf_probe_read_kernel(&slot, sizeof(slot), (const u8 *)&mem_cgroup_stats_index + idx);
    if (slot == 0xff)
        return -1;
    return slot;
}

static __always_inline struct mem_cgroup *task_mem_cgroup(struct task_struct *tsk) {
    struct cgroup_subsys_state *css;
    int ssid;

    if (!bpf_core_enum_value_exists(enum cgroup_subsys_id, memory_cgrp_id))
        return NULL;
    ssid = bpf_core_enum_value(enum cgroup_subsys_id, memory_cgrp_id);

    css = BPF_CORE_READ(tsk, cgroups, subsys[ssid]);
    if (!css)
        return NULL;

    return container_of(css, struct mem_cgroup, css);
}

// add_cpu_pending_pages adds the stat updates of a cpu that are not flushed to memcg->vmstats yet.
// This is what the rstat flush does (state - state_prev), without updating state_prev.
static long add_cpu_pending_pages(u64 cpu, struct cgroup_memory_stats_ctx *c) {
    unsigned long offset = 0;
    if (bpf_probe_read_kernel(&offset, sizeof(offset), (const unsigned long *)&__per_cpu_offset + cpu) < 0)
        return 1;

    struct memcg_vmstats_percpu *statc = (struct memcg_vmstats_percpu *)((unsigned long)c->pcpu + offset);
#pragma unroll
    for (int i = 0; i < CGROUP_MEM_NR_STATS; i++) {
        long slot = c->slot[i];
        if (slot < 0)
            continue;

        long cur = 0, prev = 0;
        bpf_probe_read_kernel(&cur, sizeof(cur), &statc->state[slot]);
        bpf_probe_read_kernel(&prev, sizeof(prev), &statc->state_prev[slot]);
        c->pages[i] += cur - prev;
    }
    return 0;
}

// page_state_bytes returns the value of a stat item in bytes. Like the kernel, it returns -1 for an
// item memcg does not track, and clamps the counters that per-cpu updates make transiently negative.
static __always_inline __u64 page_state_bytes(struct cgroup_memory_stats_ctx *c, int i) {
    if (c->slot[i] < 0)
        return (__u64)-1;
    return c->pages[i] > 0 ? (__u64)c->pages[i] * PAGE_SIZE : 0;
}

// read_cgroup_memory_pages computes the value of the stat items into c->pages: the counts flushed to
// the cgroup, the counts flushed from its descendants that are not propagated to it yet, and the
// per-cpu updates since the last flush. Each flush step moves counts between them, which keeps
// their sum, but a step overlapping the reads can have its counts read twice or missed, so it
// returns false when a flush step ran during the reads.
static __always_inline bool read_cgroup_memory_pages(struct memcg_vmstats *vmstats, struct cgroup_memory_stats_ctx *c,
                                                     u32 nr_cpus, __u64 *seq) {
    __u64 seq_begin = READ_ONCE(*seq);
    if (seq_begin & 1)
        return false;

#pragma unroll
    for (int i = 0; i < CGROUP_MEM_NR_STATS; i++) {
        long slot = c->slot[i];
        if (slot < 0)
            continue;

        long state = 0, pending = 0;
        bpf_probe_read_kernel(&state, sizeof(state), &vmstats->state[slot]);
        if (bpf_core_field_exists(vmstats->state_pending))
            bpf_probe_read_kernel(&pending, sizeof(pending), &vmstats->state_pending[slot]);
        c->pages[i] = state + pending;
    }

    if (nr_cpus)
        bpf_loop(nr_cpus, add_cpu_pending_pages, c, 0);

    return READ_ONCE(*seq) == seq_begin;
}

// get_cgroup_memory_stats fills s->mem with the memory usage of the memcg of tsk, and the value of
// some of its stat items, like memory.current and memory.stat do. Stat items are read from
// memcg->vmstats, which is only updated when the stats are flushed, so the
// per-cpu updates since the last flush are added to it. Flushing from here is not possible:
// bpf_mem_cgroup_flush_stats requires sleepable programs, and oom_kill_process cannot have sleepable programs.
// The updates of descendant cgroups that were not flushed are not accounted.
static __always_inline void get_cgroup_memory_stats(struct task_struct *tsk, struct oom_stats *s) {
    // memcg->vmstats is a pointer since 6.1, and holds the aggregated counters
    if (!bpf_core_type_exists(struct memcg_vmstats) ||
        bpf_core_field_size(struct mem_cgroup, vmstats) != sizeof(struct memcg_vmstats *) ||
        !bpf_core_field_exists(struct memcg_vmstats_percpu, state_prev))
        return;

    struct mem_cgroup *memcg = task_mem_cgroup(tsk);
    if (!memcg)
        return;

    s->mem.usage = (unsigned long)BPF_CORE_READ(memcg, memory.usage.counter) * PAGE_SIZE;

    struct memcg_vmstats *vmstats = BPF_CORE_READ(memcg, vmstats);
    if (!vmstats)
        return;

    // stat item values change between kernel versions, so resolve them against the running kernel
    struct cgroup_memory_stats_ctx c = {
        .pcpu = BPF_CORE_READ(memcg, vmstats_percpu),
        .slot = {
            [CGROUP_MEM_ACTIVE_ANON] = mem_cgroup_stats_slot(bpf_core_enum_value(enum node_stat_item, NR_ACTIVE_ANON)),
            [CGROUP_MEM_INACTIVE_ANON] = mem_cgroup_stats_slot(bpf_core_enum_value(enum node_stat_item, NR_INACTIVE_ANON)),
            [CGROUP_MEM_ACTIVE_FILE] = mem_cgroup_stats_slot(bpf_core_enum_value(enum node_stat_item, NR_ACTIVE_FILE)),
            [CGROUP_MEM_INACTIVE_FILE] = mem_cgroup_stats_slot(bpf_core_enum_value(enum node_stat_item, NR_INACTIVE_FILE)),
            [CGROUP_MEM_UNEVICTABLE] = mem_cgroup_stats_slot(bpf_core_enum_value(enum node_stat_item, NR_UNEVICTABLE)),
            [CGROUP_MEM_KERNEL] = mem_cgroup_stats_slot(bpf_core_enum_value(enum memcg_stat_item, MEMCG_KMEM)),
        },
    };

    u32 nr_cpus = 0;
    if (c.pcpu && &__per_cpu_offset && &nr_cpu_ids && bpf_helper_exists(BPF_FUNC_loop))
        bpf_probe_read_kernel(&nr_cpus, sizeof(nr_cpus), &nr_cpu_ids);

    __u64 *seq = memcg_flush_seq_ptr();
    if (!seq)
        return;

    s->mem.flags = CGROUP_MEM_STATS_UNSTABLE;
#pragma unroll
    for (int attempt = 0; attempt < CGROUP_MEM_STATS_ATTEMPTS; attempt++) {
        if (read_cgroup_memory_pages(vmstats, &c, nr_cpus, seq)) {
            s->mem.flags = 0;
            break;
        }
    }

    s->mem.active_anon = page_state_bytes(&c, CGROUP_MEM_ACTIVE_ANON);
    s->mem.inactive_anon = page_state_bytes(&c, CGROUP_MEM_INACTIVE_ANON);
    s->mem.active_file = page_state_bytes(&c, CGROUP_MEM_ACTIVE_FILE);
    s->mem.inactive_file = page_state_bytes(&c, CGROUP_MEM_INACTIVE_FILE);
    s->mem.unevictable = page_state_bytes(&c, CGROUP_MEM_UNEVICTABLE);
    s->mem.kernel = page_state_bytes(&c, CGROUP_MEM_KERNEL);
}
#endif // COMPILE_CORE

static __always_inline int handle_oom_kill(struct oom_control *oc) {
    struct oom_stats new = {};
    struct stats_key k = {
        .ts = bpf_ktime_get_ns(),
        .pid_tgid = bpf_get_current_pid_tgid(),
        .cpu = bpf_get_smp_processor_id(),
    };
    u32 pid = k.pid_tgid >> 32;
    new.trigger_pid = pid;

    struct task_struct *p = (struct task_struct *)BPF_CORE_READ(oc, chosen);
    if (!p) {
        return 0;
    }

    get_cgroup_name_for_task(p, new.cgroup_name, sizeof(new.cgroup_name));
    BPF_CORE_READ_INTO(&new.victim_pid, p, pid);
    BPF_CORE_READ_INTO(&new.score, oc, chosen_points);
#ifdef COMPILE_CORE
    if (bpf_core_field_exists(p->signal->oom_score_adj)) {
        BPF_CORE_READ_INTO(&new.score_adj, p, signal, oom_score_adj);
    }
#else
    struct signal_struct *sig;
    bpf_probe_read_kernel(&sig, sizeof(sig), &p->signal);
    bpf_probe_read_kernel(&new.score_adj, sizeof(new.score_adj), &sig->oom_score_adj);
#endif
    if (bpf_helper_exists(BPF_FUNC_get_current_comm)) {
        bpf_get_current_comm(new.trigger_comm, sizeof(new.trigger_comm));
    }

    BPF_CORE_READ_INTO(&new.victim_comm, p, comm);
    new.victim_comm[TASK_COMM_LEN - 1] = 0;

    struct mem_cgroup *memcg = NULL;
#ifdef COMPILE_CORE
    if (bpf_core_field_exists(oc->totalpages)) {
        BPF_CORE_READ_INTO(&new.pages, oc, totalpages);
    }
    if (bpf_core_field_exists(oc->memcg)) {
        BPF_CORE_READ_INTO(&memcg, oc, memcg);
    }
#else
    bpf_probe_read_kernel(&new.pages, sizeof(new.pages), &oc->totalpages);
    bpf_probe_read_kernel(&memcg, sizeof(memcg), &oc->memcg);
#endif

    new.memcg_oom = memcg != NULL ? 1 : 0;

#ifdef COMPILE_CORE
    if (new.memcg_oom && emit_cgroup_memory_stats())
        get_cgroup_memory_stats(p, &new);
#endif

    bpf_map_update_elem(&oom_stats, &k, &new, BPF_NOEXIST);

    return 0;
}

/*
 * Two entry points for the same logic: system-probe loads the fentry one when the kernel supports
 * fentry on oom_kill_process, and the kprobe one otherwise.
 */

SEC("fentry/oom_kill_process")
int BPF_PROG(fentry__oom_kill_process, struct oom_control *oc) {
    return handle_oom_kill(oc);
}

SEC("kprobe/oom_kill_process")
int BPF_KPROBE(kprobe__oom_kill_process, struct oom_control *oc) {
    return handle_oom_kill(oc);
}

char _license[] SEC("license") = "GPL";
