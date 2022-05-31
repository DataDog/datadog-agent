#ifndef __CGROUP_H__
#define __CGROUP_H__

#include <linux/version.h>

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 10, 0)

#include <linux/bpf.h>
#include <linux/cgroup.h>

#define CONTAINER_ID_LEN 64

typedef char cgroup_name_t[CONTAINER_ID_LEN+1];

struct bpf_map_def SEC("maps/cgroup_names") cgroup_names = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(cgroup_name_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

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

    return bpf_probe_read_str(buf, sz, (void *)name);
}

#endif // LINUX_VERSION_CODE

static __always_inline void update_cgroup_name() {
#if defined(CONFIG_CGROUPS) && LINUX_VERSION_CODE >= KERNEL_VERSION(4, 10, 0)
    cgroup_name_t name;
    long rc = get_cgroup_name(name, sizeof(name));
    // should have read exactly CONTAINER_ID_LEN + NUL
    if (rc != CONTAINER_ID_LEN+1 || name[CONTAINER_ID_LEN] != '\0') {
        return;
    }

    u32 pid = bpf_get_current_pid_tgid() >> 32;
    bpf_map_update_elem(&cgroup_names, &pid, &name, BPF_ANY);
#endif // defined(CONFIG_CGROUPS) && LINUX_VERSION_CODE >= KERNEL_VERSION(4, 10, 0)
}


#endif // __CGROUP_H__
