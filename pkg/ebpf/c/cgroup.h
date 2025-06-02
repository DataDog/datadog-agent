#ifndef BPF_CGROUP_H
#define BPF_CGROUP_H

#ifdef COMPILE_RUNTIME
#include <linux/cgroup.h>
#endif

#include "bpf_core_read.h"
#include "bpf_tracing.h"
#include "bpf_builtins.h"

static __always_inline int get_cgroup_name_for_task(struct task_struct *task, char *buf, size_t sz) {
    bpf_memset(buf, 0, sz);

    #ifdef COMPILE_CORE
        enum cgroup_subsys_id___local {
            memory_cgrp_id___local = 123, /* value doesn't matter */
        };
        int cgrp_id = bpf_core_enum_value(enum cgroup_subsys_id___local, memory_cgrp_id___local);
    #else
        int cgrp_id = memory_cgrp_id;
    #endif

    const char *name = BPF_CORE_READ(task, cgroups, subsys[cgrp_id], cgroup, kn, name);
    if (bpf_probe_read_kernel(buf, sz, name) < 0) {
        return 0;
    }
    return 1;
}

static __always_inline int get_cgroup_name(char *buf, size_t sz) {
    if (!bpf_helper_exists(BPF_FUNC_get_current_task)) {
        return 0;
    }

    struct task_struct *cur_tsk = (struct task_struct *)bpf_get_current_task();
    return get_cgroup_name_for_task(cur_tsk, buf, sz);
}

#endif /* defined(BPF_CGROUP_H) */
