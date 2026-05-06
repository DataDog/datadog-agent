#ifndef __NOISY_NEIGHBOR_KERN_USER_H
#define __NOISY_NEIGHBOR_KERN_USER_H

#include "ktypes.h"

typedef struct {
    __u64 sum_latencies_ns;
    __u64 event_count;
    __u64 preemption_count;
    __u64 pid_count;
    __u64 sum_cycles;
    __u64 sum_instructions;
    __u64 sum_llc_misses;
    __u64 sum_itlb_misses;
    __u64 sum_softirq_ns;
    __u64 block_io_requests;
} cgroup_agg_stats_t;

typedef struct {
    __u64 cycles;
    __u64 instructions;
    __u64 llc_misses;
    __u64 itlb_misses;
} task_pmu_stamp_t;

#endif
