#ifndef __CGROUP_H__
#define __CGROUP_H__

#include <linux/version.h>

#include "bpf_helpers.h"

#include <linux/bpf.h>
#include <linux/cgroup.h>

#define CONTAINER_ID_LEN 64

#define CGROUP_ID_NOT_FOUND ((u64)-1)

#if LINUX_VERSION_CODE >= KERNEL_VERSION(4, 10, 0)

typedef char cgroup_name_t[CONTAINER_ID_LEN+1];

struct bpf_map_def SEC("maps/cgroup_names") cgroup_names = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(u64),
    .value_size = sizeof(cgroup_name_t),
    .max_entries = 1024,
    .pinning = 0,
    .namespace = "",
};

struct _cgroup_t {
    cgroup_name_t name;
    u64 id;
};

static __always_inline u8 is_container_id(cgroup_name_t name) {
#pragma unroll
    for (int n = 0; n < sizeof(cgroup_name_t); ++n) {
        char c = name[n];
        if (c == '\0' ||
            (c >= '0' && c <= '9') ||
            (c >= 'a' && c <= 'f') ||
            (c >= 'A' && c <= 'F')) {
            continue;
        }

        return 0;
    }

    return 1;
}

static __always_inline int get_cgroup(struct _cgroup_t * cg) {
    __builtin_memset(cg->name, 0, sizeof(cg->name));

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

    int copied = bpf_probe_read_str(cg->name, sizeof(cg->name), (void *)name);
    if (copied != CONTAINER_ID_LEN+1 || cg->name[CONTAINER_ID_LEN] != '\0') {
        return -1;
    }

    if (!is_container_id(cg->name)) {
        return -1;
    }

    int level;
    if (bpf_probe_read(&level, sizeof(level), &(cgrp->level)) < 0) {
        return -1;
    }

    return bpf_probe_read(&(cg->id), sizeof(cg->id), &(cgrp->ancestor_ids[level]));
}

#endif // LINUX_VERSION_CODE

#define GET_CGROUP_ID get_cgroup_id

static __always_inline u64 get_cgroup_id() {
#if defined(CONFIG_CGROUPS) && LINUX_VERSION_CODE >= KERNEL_VERSION(4, 10, 0)
    struct _cgroup_t cg = {};
    if (get_cgroup(&cg) < 0 || cg.id == CGROUP_ID_NOT_FOUND) {
        return CGROUP_ID_NOT_FOUND;
    }

    log_debug("cgroup id=%d name=%s\n", cg.id, cg.name);

    bpf_map_update_elem(&cgroup_names, &cg.id, &cg.name, BPF_ANY);
    return cg.id;
#else
    return 0;
#endif // defined(CONFIG_CGROUPS) && LINUX_VERSION_CODE >= KERNEL_VERSION(4, 10, 0)
}


#endif // __CGROUP_H__
