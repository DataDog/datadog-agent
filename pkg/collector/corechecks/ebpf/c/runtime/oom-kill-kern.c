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

extern int LINUX_KERNEL_VERSION __kconfig;

// emit_cgroup_memory_stats is set by system-probe from oom_kill.emit_cgroup_memory_stats. It is
// patched into the instructions, rather than kept in .rodata, so that it is part of the program tag
// and a pinned program built with a different value is not taken over.
static __always_inline bool emit_cgroup_memory_stats() {
    __u64 val = 0;
    LOAD_CONSTANT("emit_cgroup_memory_stats", val);
    return val == 1;
}

#ifdef COMPILE_CORE
static __always_inline void get_cgroup_memory_stats__v7_0(struct task_struct* tsk, struct oom_stats* s) {
    // struct mem_cgroup is only used through pointers, and is first reached through pointer members of
    // other structs (e.g. oom_control.memcg), so clang would emit it to BTF as a forward declaration.
    // The loader then rejects the mem_cgroup kfuncs, because their prototypes in the kernel BTF use the
    // full struct. Referencing the complete type here makes clang emit it.
    if (!bpf_core_type_exists(struct mem_cgroup))
        return;

    struct cgroup *cgrp = tsk->cgroups->dfl_cgrp;
    struct mem_cgroup *memcg;
    memcg = bpf_get_mem_cgroup(&cgrp->self);
    if (!memcg)
        return;

    // flushing stats via `bpf_mem_cgroup_flush_stats` requires making the ebpf program
    // sleepable which limits what we can do from within it. Since the oom killer is invoked
    // after the pagescan in the memory reclaim process which already flushes stats, and since
    // we do not expect any more allocations to happen for the victim process, it is fine to continue
    // without this flush
    s->mem.usage = bpf_mem_cgroup_usage(memcg);
    s->mem.active_anon = bpf_mem_cgroup_page_state(memcg, NR_ACTIVE_ANON);
    s->mem.inactive_anon = bpf_mem_cgroup_page_state(memcg, NR_INACTIVE_ANON);
    s->mem.active_file = bpf_mem_cgroup_page_state(memcg, NR_ACTIVE_FILE);
    s->mem.inactive_file = bpf_mem_cgroup_page_state(memcg, NR_INACTIVE_FILE);
    s->mem.unevictable = bpf_mem_cgroup_page_state(memcg, NR_UNEVICTABLE);
    s->mem.kernel = bpf_mem_cgroup_page_state(memcg, MEMCG_KMEM);

    bpf_put_mem_cgroup(memcg);

    return;
}

#ifndef PAGE_SIZE
#define PAGE_SIZE 4096
#endif

static __always_inline unsigned long mem_cgroup_usage(struct mem_cgroup* memcg) {
    unsigned long pages = (unsigned long)BPF_CORE_READ(memcg, memory.usage.counter);
    return pages * PAGE_SIZE;
}

// Since 6.10 (ff48c71c26aa) memcg_vmstats only holds the stat items tracked by memcg, and the kernel
// maps a stat item to its slot through this table, which is not in BTF.
extern const void mem_cgroup_stats_index __ksym __weak;

// mem_cgroup_stats_slot returns the slot of the stat item idx in memcg->vmstats->state, or -1 if
// memcg does not track it.
static __always_inline long mem_cgroup_stats_slot(int idx) {
    if (idx < 0 || idx >= bpf_core_enum_value(enum memcg_stat_item, MEMCG_NR_STAT))
        return -1;

    if (LINUX_KERNEL_VERSION < KERNEL_VERSION(6, 10, 0))
        return idx;

    if (!&mem_cgroup_stats_index)
        return -1;

    u8 slot = 0xff;
    bpf_probe_read_kernel(&slot, sizeof(slot), (const u8 *)&mem_cgroup_stats_index + idx);
    if (slot == 0xff)
        return -1;
    return slot;
}

// mem_cgroup_page_state mirrors bpf_mem_cgroup_page_state for kernels without the kfunc: the value
// of the stat item idx (memcg->vmstats->state), in bytes. Like the kernel, it returns -1 for an
// item memcg does not track. 
static __always_inline unsigned long mem_cgroup_page_state(struct mem_cgroup *memcg, int idx) {
    long slot = mem_cgroup_stats_slot(idx);
    if (slot < 0)
        return (unsigned long)-1;

    struct memcg_vmstats *vmstats = BPF_CORE_READ(memcg, vmstats);
    if (!vmstats)
        return 0;

    long pages = 0;
    bpf_probe_read_kernel(&pages, sizeof(pages), &vmstats->state[slot]);
    if (pages < 0)
        pages = 0;
    return (unsigned long)pages * PAGE_SIZE;
}

static __always_inline struct mem_cgroup *task_mem_cgroup(struct task_struct *tsk)
{
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

static __always_inline void get_cgroup_memory_stats__v6_8(struct task_struct* tsk, struct oom_stats* s) {
    struct mem_cgroup* memcg = task_mem_cgroup(tsk);
    if (!memcg)
        return;

    // stat item values change between kernel versions, so resolve them against the running kernel
    s->mem.usage = mem_cgroup_usage(memcg);
    s->mem.active_anon = mem_cgroup_page_state(memcg, bpf_core_enum_value(enum node_stat_item, NR_ACTIVE_ANON));
    s->mem.inactive_anon = mem_cgroup_page_state(memcg, bpf_core_enum_value(enum node_stat_item, NR_INACTIVE_ANON));
    s->mem.active_file = mem_cgroup_page_state(memcg, bpf_core_enum_value(enum node_stat_item, NR_ACTIVE_FILE));
    s->mem.inactive_file = mem_cgroup_page_state(memcg, bpf_core_enum_value(enum node_stat_item, NR_INACTIVE_FILE));
    s->mem.unevictable = mem_cgroup_page_state(memcg, bpf_core_enum_value(enum node_stat_item, NR_UNEVICTABLE));
    s->mem.kernel = mem_cgroup_page_state(memcg, bpf_core_enum_value(enum memcg_stat_item, MEMCG_KMEM));

    return;
}

static __always_inline void get_cgroup_memory_stats(struct task_struct* tsk, struct oom_stats* s) {
    if (LINUX_KERNEL_VERSION >= KERNEL_VERSION(7, 0, 0)) {
        s32 pid = 0;
        BPF_CORE_READ_INTO(&pid, tsk, pid);
        
        struct task_struct* acq = bpf_task_from_vpid(pid);
        if (!acq)
            return;

        get_cgroup_memory_stats__v7_0(acq, s);
        bpf_task_release(acq);
    } else if (LINUX_KERNEL_VERSION >= KERNEL_VERSION(6, 8, 0)) {
        return get_cgroup_memory_stats__v6_8(tsk, s);
    }

    return;
}
#endif // COMPILE_CORE

static __always_inline int handle_oom_kill(struct oom_control *oc, bool collect_cgroup_stats) {
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
    if (new.memcg_oom && collect_cgroup_stats && emit_cgroup_memory_stats())
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
    return handle_oom_kill(oc, true);
}

SEC("kprobe/oom_kill_process")
int BPF_KPROBE(kprobe__oom_kill_process, struct oom_control *oc) {
    // kprobes do not allow a call to `bpf_task_from_vpid` so we cannot
    // collect cgroup memory stats when using this attach type
    return handle_oom_kill(oc, false);
}

char _license[] SEC("license") = "GPL";
