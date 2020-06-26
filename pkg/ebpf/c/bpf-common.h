#ifndef BPF_COMMON_H
#define BPF_COMMON_H

#include <linux/bpf.h>
#include <linux/cgroup.h>

static inline __attribute__((always_inline))
int get_cgroup_name(char *buf, size_t sz) {
    struct task_struct *cur_tskd = (struct task_struct *)bpf_get_current_task();
    struct css_set *css_set;
    if (!bpf_probe_read(&css_set, sizeof(css_set), &cur_tskd->cgroups)) {
      struct cgroup_subsys_state *css;
      // TODO: Do not arbitrarily pick the first subsystem
      if (!bpf_probe_read(&css, sizeof(css), &css_set->subsys[0])) {
        struct cgroup *cgrp;
        if (!bpf_probe_read(&cgrp, sizeof(cgrp), &css->cgroup)) {
          struct kernfs_node *kn;
          if (!bpf_probe_read(&kn, sizeof(kn), &cgrp->kn)) {
            if (!bpf_probe_read_str(buf, sz, &kn->name)) {
              return 0;
            }
          }
        }
      }
    }
    return -1;
}

#endif /* defined(BPF_COMMON_H) */
