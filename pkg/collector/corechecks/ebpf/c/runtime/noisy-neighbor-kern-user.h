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
    __u64 sum_cache_misses;
    __u64 sum_itlb_misses;
    __u64 sum_softirq_ns;
    __u64 block_io_requests;
    __u64 sum_branch_misses;
    __u64 sum_cpu_migrations;
    __u64 wakeup_count;
    __u64 sum_cache_references;
} cgroup_agg_stats_t;

// Per-PMU-event stamp: counter value plus enabled/running times.
// enabled and running are needed to scale counter deltas back up when the
// kernel time-multiplexes a hardware counter onto fewer physical counters
// than there are configured events. Without scaling, multiplexed events
// under-report by a factor of running/enabled. See bpf_perf_event_read_value
// docs and tools/perf/Documentation/perf-stat.txt for the standard formula
// scaled = counter * enabled / running.
typedef struct {
    __u64 counter;
    __u64 enabled;
    __u64 running;
} pmu_event_stamp_t;

typedef struct {
    pmu_event_stamp_t cycles;
    pmu_event_stamp_t instructions;
    pmu_event_stamp_t cache_misses;
    pmu_event_stamp_t itlb_misses;
    pmu_event_stamp_t branch_misses;
    pmu_event_stamp_t cpu_migrations;
    pmu_event_stamp_t cache_references;
} task_pmu_stamp_t;

#endif
