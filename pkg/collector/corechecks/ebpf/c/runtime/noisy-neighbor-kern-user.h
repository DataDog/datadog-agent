#ifndef __NOISY_NEIGHBOR_KERN_USER_H
#define __NOISY_NEIGHBOR_KERN_USER_H

#include "ktypes.h"

typedef struct {
    __u64 sum_latencies_ns;
    __u64 event_count;
    __u64 foreign_preemption_count;
    __u64 self_preemption_count;
    __u64 pid_count;
    __u64 latency_bucket_lt_100us;
    __u64 latency_bucket_100us_1ms;
    __u64 latency_bucket_1ms_10ms;
    __u64 latency_bucket_gt_10ms;
} cgroup_agg_stats_t;

#endif
