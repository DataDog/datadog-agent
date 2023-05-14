#include "ktypes.h"

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
    get_cgroup_name(s->cgroup_name, sizeof(s->cgroup_name));

    struct task_struct *p = (struct task_struct *)BPF_CORE_READ(oc, chosen);
    if (!p) {
        return 0;
    }
    BPF_CORE_READ_INTO(&s->tpid, p, pid);

    if (bpf_helper_exists(BPF_FUNC_get_current_comm)) {
        bpf_get_current_comm(&s->fcomm, sizeof(s->fcomm));
    }
    if (bpf_helper_exists(BPF_FUNC_probe_read_str)) {
        BPF_CORE_READ_STR_INTO(&s->tcomm, p, comm);
    } else {
        BPF_CORE_READ_INTO(&s->tcomm, p, comm);
        s->tcomm[TASK_COMM_LEN - 1] = 0;
    }

    struct mem_cgroup *memcg = NULL;
#ifdef COMPILE_CORE
    if (bpf_core_field_exists(oc->totalpages)) {
        BPF_CORE_READ_INTO(&s->pages, oc, totalpages);
    }
    if (bpf_core_field_exists(oc->memcg)) {
        BPF_CORE_READ_INTO(&memcg, oc, memcg);
    }
#else
    bpf_probe_read_kernel(&s->pages, sizeof(s->pages), &oc->totalpages);
    bpf_probe_read_kernel(&memcg, sizeof(memcg), &oc->memcg);
#endif

    s->memcg_oom = memcg != NULL ? 1 : 0;
    return 0;
}

__u32 _version SEC("version") = 0xFFFFFFFE;
char _license[] SEC("license") = "GPL";
