#ifndef __NOISY_NEIGHBOR_KERN_USER_H
#define __NOISY_NEIGHBOR_KERN_USER_H

#include "ktypes.h"

typedef struct {
    __u64 sum_latencies_ns;
    __u64 event_count;
    __u64 preemption_count;
    __u64 pid_count;
} cgroup_agg_stats_t;

#define NOISY_NEIGHBOR_HARDWARE_EVENTS 6

typedef struct {
    __u64 value;
    __u64 enabled;
    __u64 running;
} pmu_counter_t;

typedef struct {
    pmu_counter_t counters[NOISY_NEIGHBOR_HARDWARE_EVENTS];
    __u64 cpu_migrations;
    __u64 sampled_event_mask;
} pmu_cgroup_stats_t;

typedef struct {
    __u32 active;
    __u32 _padding;
    __u64 generation;
    __u64 effective_event_mask;
} pmu_config_t;

typedef struct {
    __u64 read_errors;
} pmu_error_stats_t;

#endif
