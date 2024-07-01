#pragma once

#include "ktypes.h"

typedef struct {
    unsigned int x, y, z;
} dim3;

typedef struct {
    __u64 pid_tgid;
    __u64 kernel_addr;
    __u16 probe_id;
    __u64 stream_id;
    __u64 ktime_ns;
    __u64 shared_mem_size;
    dim3 grid_size;
    dim3 block_size;
} cuda_kernel_launch_t;
