#include "oom-kill-kern-user.h"
#include "bpf-common.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"

/*
 * The `oom_stats` hash map is used to share with the userland program system-probe
 * the statistics per pid
 */

BPF_HASH_MAP(oom_stats, u32, struct oom_stats, 10240)

SEC("kprobe/oom_kill_process")
int BPF_KPROBE(kprobe__oom_kill_process, struct oom_control *oc) {
    struct oom_stats zero = {};
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    bpf_map_update_elem(&oom_stats, &pid, &zero, BPF_NOEXIST);
    struct oom_stats *s = bpf_map_lookup_elem(&oom_stats, &pid);
    if (!s) {
        return 0;
    }

    s->pid = pid;

    struct task_struct *p;
    bpf_probe_read(&p, sizeof(p), &oc->chosen);
    bpf_probe_read(&s->tpid, sizeof(s->tpid), &p->pid);

    get_cgroup_name(s->cgroup_name, sizeof(s->cgroup_name));

    BPF_CORE_READ_INTO(&s->tpid, oc, chosen, pid);

    if (bpf_core_enum_value_exists(enum bpf_func_id, BPF_FUNC_get_current_comm)) {
        bpf_get_current_comm(&s->fcomm, sizeof(s->fcomm));
    }

    if (bpf_core_enum_value_exists(enum bpf_func_id, BPF_FUNC_probe_read_str)) {
        BPF_CORE_READ_STR_INTO(&s->tcomm, oc, chosen, comm);
    } else {
        BPF_CORE_READ_INTO(&s->tcomm, oc, chosen, comm);
        s->tcomm[TASK_COMM_LEN - 1] = 0;
    }

    if (bpf_core_field_exists(oc->totalpages)) {
        BPF_CORE_READ_INTO(&s->pages, oc, totalpages);
    }

    if (bpf_core_field_exists(oc->memcg)) {
        struct mem_cgroup *memcg = NULL;
        memcg = BPF_CORE_READ(oc, memcg);
        s->memcg_oom = memcg != NULL ? 1 : 0;
    }

    return 0;
}

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
