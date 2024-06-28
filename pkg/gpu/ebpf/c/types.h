#pragma once

#include "ktypes.h"

typedef struct {
    __u64 pid_tgid;
    __u64 kernel_addr;
    __u16 probe_id;
    __u64 stream_id;
    __u64 ktime_ns;
    __u64 grid_size;
    __u64 block_size;
    __u64 shared_mem_size;
} cuda_kernel_launch_t;

struct dim3 {
    unsigned int x, y, z;
};
