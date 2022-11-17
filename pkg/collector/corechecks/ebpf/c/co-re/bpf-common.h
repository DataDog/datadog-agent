#ifndef BPF_COMMON_H
#define BPF_COMMON_H

#include "vmlinux.h"
#include "bpf_core_read.h"
#include "bpf_helpers.h"

enum cgroup_subsys_id___local {
		memory_cgrp_id___local = 123, /* value doesn't matter */
};

static __always_inline int get_cgroup_name(char *buf, size_t sz) {
    __builtin_memset(buf, 0, sz);

    if (!bpf_core_enum_value_exists(enum bpf_func_id, BPF_FUNC_get_current_task)) {
        return -1;
    }

    struct task_struct *cur_tsk = (struct task_struct *)bpf_get_current_task();

    int cgrp_id = bpf_core_enum_value(enum cgroup_subsys_id___local, memory_cgrp_id___local);
    const char *name = BPF_CORE_READ(cur_tsk, cgroups, subsys[cgrp_id], cgroup, kn, name);
    if (bpf_probe_read_kernel_str(buf, sz, name) < 0) {
        return -1;
    }

    return 0;
}

#endif /* defined(BPF_COMMON_H) */
