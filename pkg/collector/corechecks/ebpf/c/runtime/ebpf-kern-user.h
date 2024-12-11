#ifndef __EBPF_KERN_USER_H
#define __EBPF_KERN_USER_H

#include "ktypes.h"

typedef struct {
    __u32 map_id;
    __u32 cpu;
} perf_buffer_key_t;

typedef struct {
    unsigned long len;
    long addr;
} mmap_region_t;

typedef struct {
    mmap_region_t consumer;
    mmap_region_t data;
} ring_mmap_t;

#endif
