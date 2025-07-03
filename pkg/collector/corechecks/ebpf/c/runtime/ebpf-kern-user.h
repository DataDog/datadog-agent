#ifndef __EBPF_KERN_USER_H
#define __EBPF_KERN_USER_H

#include "ktypes.h"

#define EBPF_CHECK_KPROBE_MISSES_CMD 0x70C14

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

typedef struct {
    unsigned int kprobe_id;
    unsigned int query_id;
} cookie_t;

typedef struct {
    unsigned long kprobe_hits;
    unsigned long kprobe_nesting_misses;
    unsigned long kretprobe_maxactive_misses;
} kprobe_stats_t;

typedef enum {
    FILE_NOT_PERF_EVENT                         = 1,
    PERF_EVENT_FD_IS_NOT_KPROBE                 = 2,
    PERF_EVENT_NOT_FOUND                        = 3,
    ERR_READING_PERF_PMU                        = 4,
    ERR_READING_KPROBE_HITS                     = 5,
    ERR_READING_KPROBE_MISSES                   = 6,
    ERR_READING_KRETPROBE_MISSES                = 7,
    ERR_READING_TRACE_EVENT_CALL_FLAGS          = 8,
    ERR_READING_TRACEFS_KPROBE                  = 9,
    ERR_READING_TRACE_KPROBE_FROM_PERF_EVENT    = 10,
} stats_collector_error_t;

typedef struct {
    stats_collector_error_t error_type;
    cookie_t cookie;
} k_stats_error_t;

#endif
