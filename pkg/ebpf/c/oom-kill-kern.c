#define KBUILD_MODNAME "foo"
#include <linux/bpf.h>
#include <linux/cgroup.h>
#include <linux/oom.h>

#include "pkg/ebpf/c/oom-kill-kern-user.h"


/*
 * The `oomStats` hash map is used to share with the userland program system-probe
 * the statistics per pid
 */
BPF_HASH(oomStats, u32, struct oom_stats);

static inline __attribute__((always_inline))
bool is_memcg_oom(struct oom_control *oc)
{
  return oc->memcg != NULL;
}

int kprobe__oom_kill_process(struct pt_regs *ctx, struct oom_control *oc, const char *message) {
    struct oom_stats zero = {};
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    struct oom_stats *s = oomStats.lookup_or_init(&pid, &zero);
    if (s == NULL) return 0;

    struct task_struct *cur_tsk = (struct task_struct *)bpf_get_current_task();
    struct css_set *css_set;
    if (!bpf_probe_read(&css_set, sizeof(css_set), &cur_tsk->cgroups)) {
      struct cgroup_subsys_state *css;
      // TODO: Do not arbitrarily pick the first subsystem
      if (!bpf_probe_read(&css, sizeof(css), &css_set->subsys[0])) {
        struct cgroup *cgrp;
        if (!bpf_probe_read(&cgrp, sizeof(cgrp), &css->cgroup)) {
          struct kernfs_node *kn;
          if (!bpf_probe_read(&kn, sizeof(kn), &cgrp->kn)) {
            const char *name;
            if (!bpf_probe_read(&name, sizeof(name), &kn->name)) {
              bpf_probe_read_str(&s->cgroup_name, sizeof(s->cgroup_name), name);
            }
          }
        }
      }
    }

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
