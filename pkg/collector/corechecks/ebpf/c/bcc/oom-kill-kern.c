#define KBUILD_MODNAME "ddsysprobe"
#include <linux/oom.h>

#include "bpf-common.h"
#include "oom-kill-kern-user.h"


/*
 * The `oomStats` hash map is used to share with the userland program system-probe
 * the statistics per pid
 */
BPF_HASH(oomStats, u32, struct oom_stats);

int kprobe__oom_kill_process(struct pt_regs *ctx, struct oom_control *oc, const char *message) {
    struct oom_stats zero = {};
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    struct oom_stats *s = oomStats.lookup_or_init(&pid, &zero);
    if (s == NULL) return 0;

    // From bpf-common.h
    get_cgroup_name(s->cgroup_name, sizeof(s->cgroup_name));

    struct task_struct *p = oc->chosen;
    unsigned long totalpages;

    s->pid = pid;
    s->tpid = p->pid;
    bpf_get_current_comm(&s->fcomm, sizeof(s->fcomm));
    bpf_probe_read_str(&s->tcomm, sizeof(s->tcomm), p->comm);
    s->pages = oc->totalpages;

    #if LINUX_VERSION_CODE >= KERNEL_VERSION(4,8,0)
      s->memcg_oom = oc->memcg != NULL ? 1 : 0;
    #endif

    return 0;
}
