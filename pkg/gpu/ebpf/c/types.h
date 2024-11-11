#ifndef __GPU_TYPES_H
#define __GPU_TYPES_H

#include "ktypes.h"

typedef struct {
    unsigned int x, y, z;
} dim3;

typedef enum {
    cuda_kernel_launch,
    cuda_memory_event,
    cuda_sync,
    cuda_set_device,
} cuda_event_type_t;

#define MAX_CONTAINER_ID_LEN 129

typedef struct {
    cuda_event_type_t type;
    __u64 pid_tgid;
    __u64 stream_id;
    __u64 ktime_ns;
    char cgroup[MAX_CONTAINER_ID_LEN];
} cuda_event_header_t;

typedef struct {
    cuda_event_header_t header;
} cuda_sync_t;

typedef struct {
    cuda_event_header_t header;
    __u64 kernel_addr;
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

typedef struct {
    __u64 size;
    void **devPtr;
} cuda_alloc_request_args_t;

typedef struct {
    cuda_event_header_t header;
    int device;
} cuda_set_device_event_t;

#endif
