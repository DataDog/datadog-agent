#ifndef __NOISY_NEIGHBOR_KERN_USER_H
#define __NOISY_NEIGHBOR_KERN_USER_H

#include "ktypes.h"

typedef struct {
    __u64 sum_latencies_ns;
    __u64 event_count;
    __u64 foreign_preemption_count;
    __u64 self_preemption_count;
    __u64 task_count;
    __u64 latency_bucket_lt_100us;
    __u64 latency_bucket_100us_1ms;
    __u64 latency_bucket_1ms_10ms;
    __u64 latency_bucket_gt_10ms;
    __u64 cpu_migrations;
} cgroup_agg_stats_t;

typedef struct {
    __u64 victim_cgroup_id;
    __u64 preemptor_cgroup_id;
} preemptor_key_t;

typedef struct {
    __u64 count;
} preemptor_stats_t;

#endif
