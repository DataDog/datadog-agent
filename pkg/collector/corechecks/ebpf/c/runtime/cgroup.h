#ifndef BPF_CGROUP_H
#define BPF_CGROUP_H

#ifdef COMPILE_RUNTIME
#include <linux/cgroup.h>
#endif

#include "bpf_core_read.h"
#include "bpf_tracing.h"

static __always_inline int get_cgroup_name(char *buf, size_t sz) {
    if (!bpf_helper_exists(BPF_FUNC_get_current_task)) {
        return -1;
    }
    __builtin_memset(buf, 0, sz);
    struct task_struct *cur_tsk = (struct task_struct *)bpf_get_current_task();

#ifdef COMPILE_CORE
    enum cgroup_subsys_id___local {
        memory_cgrp_id___local = 123, /* value doesn't matter */
    };
    int cgrp_id = bpf_core_enum_value(enum cgroup_subsys_id___local, memory_cgrp_id___local);
#else
    int cgrp_id = memory_cgrp_id;
#endif
    const char *name = BPF_CORE_READ(cur_tsk, cgroups, subsys[cgrp_id], cgroup, kn, name);
    if (bpf_probe_read_kernel_str(buf, sz, name) < 0) {
        return -1;
    }

    return 0;
}

#endif /* defined(BPF_CGROUP_H) */
