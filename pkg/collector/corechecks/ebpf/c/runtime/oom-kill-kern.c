#include "ktypes.h"
#include "bpf_metadata.h"

#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#include <linux/oom.h>

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

/*
 * The `oom_stats` hash map is used to share with the userland program system-probe
 * the statistics per pid
 */

BPF_HASH_MAP(oom_stats, u64, struct oom_stats, 10240)

SEC("kprobe/oom_kill_process")
int BPF_KPROBE(kprobe__oom_kill_process, struct oom_control *oc) {
    struct oom_stats zero = {};
    struct oom_stats new = {};
    u64 ts = bpf_ktime_get_ns();
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    bpf_map_update_elem(&oom_stats, &ts, &zero, BPF_NOEXIST);
    struct oom_stats *s = bpf_map_lookup_elem(&oom_stats, &ts);
    if (!s) {
        return 0;
    }

    // for kernel before 4.11 the prototype for bpf_probe_read helpers
    // expected a pointer to stack memory. Therefore, we work on stack
    // variable and update the map value at the end
    bpf_memcpy(&new, s, sizeof(struct oom_stats));
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
    bpf_probe_read_kernel(&sig, sizeof(s), &p->signal);
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

    bpf_memcpy(s, &new, sizeof(struct oom_stats));

    return 0;
}

char _license[] SEC("license") = "GPL";
