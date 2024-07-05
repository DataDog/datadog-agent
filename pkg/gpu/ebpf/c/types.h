#pragma once

#include "ktypes.h"

typedef struct {
    unsigned int x, y, z;
} dim3;

typedef enum {
    cuda_kernel_launch,
    cuda_memory_event,
    cuda_sync
} cuda_event_type_t;

typedef struct {
    cuda_event_type_t type;
    __u64 pid_tgid;
    __u64 stream_id;
} cuda_event_header_t;

typedef struct {
    cuda_event_header_t header;
    __u64 kernel_addr;
    __u64 ktime_ns;
    __u64 shared_mem_size;
    dim3 grid_size;
    dim3 block_size;
} cuda_kernel_launch_t;

typedef enum {
    cudaMalloc,
    cudaFree
} cuda_memory_event_type_t;

typedef struct {
    cuda_event_header_t header;
    __u64 size;
    __u64 addr;
    cuda_memory_event_type_t type;
} cuda_memory_event_t;
