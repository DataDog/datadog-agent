#include <linux/compiler.h>
#include <linux/kconfig.h>
#include <linux/ptrace.h>
#include <linux/types.h>
#include <linux/version.h>
#include <linux/oom.h>

#include "bpf_helpers.h"
#include "bpf-common.h"
#include "oom-kill-kern-user.h"

#if LINUX_VERSION_CODE < KERNEL_VERSION(4, 9, 0)
// 4.8 is the first version where `struct oom_control*` is the first argument of `oom_kill_process`
// 4.9 is the first version where the field `totalpages` is available in `struct oom_control`
#error Versions of Linux previous to 4.9.0 are not supported by this probe
#endif

/*
 * The `oom_stats` hash map is used to share with the userland program system-probe
 * the statistics per pid
 */

struct bpf_map_def SEC("maps/oom_stats") oom_stats = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct oom_stats),
    .max_entries = 10240,
    .pinning = 0,
    .namespace = "",
};

SEC("kprobe/oom_kill_process")
int kprobe__oom_kill_process(struct pt_regs *ctx) {
    struct oom_control *oc = PT_REGS_PARM1(ctx);

    struct oom_stats zero = {};
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    bpf_map_update_elem(&oom_stats, &pid, &zero, BPF_NOEXIST);
    struct oom_stats *s = bpf_map_lookup_elem(&oom_stats, &pid);
    if (!s) {
        return 0;
    }

    s->pid = pid;

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 8, 0)
    // From bpf-common.h
    get_cgroup_name(s->cgroup_name, sizeof(s->cgroup_name));
#endif

    struct task_struct *p;
    bpf_probe_read(&p, sizeof(p), &oc->chosen);
    bpf_probe_read(&s->tpid, sizeof(s->tpid), &p->pid);

    bpf_get_current_comm(&s->fcomm, sizeof(s->fcomm));
#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 8, 0)
    bpf_probe_read_str(&s->tcomm, sizeof(s->tcomm), (void *)&p->comm);
#else
    bpf_probe_read(&s->tcomm, sizeof(s->tcomm), (void *)&p->comm);
    s->tcomm[TASK_COMM_LEN - 1] = 0;
#endif
    bpf_probe_read(&s->pages, sizeof(s->pages), &oc->totalpages);

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 8, 0)
    struct mem_cgroup *memcg;
    bpf_probe_read(&memcg, sizeof(memcg), &oc->memcg);
    s->memcg_oom = memcg != NULL ? 1 : 0;
#endif

    return 0;
}

// This number will be interpreted by elf-loader to set the current running kernel version
__u32 _version SEC("version") = 0xFFFFFFFE; // NOLINT(bugprone-reserved-identifier)

char _license[] SEC("license") = "GPL"; // NOLINT(bugprone-reserved-identifier)
