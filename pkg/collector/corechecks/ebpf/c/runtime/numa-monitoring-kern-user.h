#ifndef __NUMA_MONITORING_KERN_USER_H
#define __NUMA_MONITORING_KERN_USER_H

#include "ktypes.h"

typedef struct {
    __u64 cgroup_id;
    __u32 numa_node;
    __u32 _padding;
} numa_runtime_key_t;

typedef struct {
    __u64 runtime_ns;
} numa_runtime_value_t;

#endif
