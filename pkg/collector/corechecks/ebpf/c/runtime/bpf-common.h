#ifndef BPF_COMMON_H
#define BPF_COMMON_H

#include <linux/bpf.h>
#include <linux/cgroup.h>

static __always_inline int get_cgroup_name(char *buf, size_t sz) {
    memset(buf, 0, sz);

    struct task_struct *cur_tsk = (struct task_struct *)bpf_get_current_task();

    struct css_set *css_set;
    if (bpf_probe_read(&css_set, sizeof(css_set), &cur_tsk->cgroups) < 0)
        return -1;

    struct cgroup_subsys_state *css;
    if (bpf_probe_read(&css, sizeof(css), &css_set->subsys[0]) < 0)
        return -1;

    struct cgroup *cgrp;
    if (bpf_probe_read(&cgrp, sizeof(cgrp), &css->cgroup) < 0)
        return -1;

    struct kernfs_node *kn;
    if (bpf_probe_read(&kn, sizeof(kn), &cgrp->kn) < 0)
        return -1;

    const char *name;
    if (bpf_probe_read(&name, sizeof(name), &kn->name) < 0)
        return -1;

    if (bpf_probe_read_str(buf, sz, (void *)name) < 0)
        return -1;

    return 0;
}

#endif /* defined(BPF_COMMON_H) */
