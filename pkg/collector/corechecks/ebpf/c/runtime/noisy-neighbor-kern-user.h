#ifndef __NOISY_NEIGHBOR_KERN_USER_H
#define __NOISY_NEIGHBOR_KERN_USER_H

#include "ktypes.h"

// Note: In eBPF/kernel code, "pid" refers to task_struct->pid which is the Thread ID (TID)
// Userspace code interprets these as TIDs for accurate thread-level scheduling metrics
typedef struct {
    __u64 sum_latencies_ns;
    __u64 event_count;
    __u64 preemption_count;
} __attribute__((packed)) cgroup_agg_stats_t;

typedef struct {
    __u64 cgroup_id;
    __u32 pid;
} __attribute__((packed)) pid_key_t;

#endif
