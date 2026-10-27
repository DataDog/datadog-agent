#ifndef OOM_KILL_KERN_USER_H
#define OOM_KILL_KERN_USER_H

#include "ktypes.h"

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif

typedef struct {} oom_stats_v1;

struct oom_stats {
    // version marker to ensure compatibility
    // between a new oom kill map vs the pinned map.
    // Bump it whenever this struct changes, see
    // pkg/collector/corechecks/ebpf/probe/oomkill/oom_stats_layout_linux.go
    oom_stats_v1 oom_stats_version;
    // Pid of killed process
    __u32 victim_pid;
    // Pid of triggering process
    __u32 trigger_pid;
    // Name of killed process
    char victim_comm[TASK_COMM_LEN];
    // Name of triggering process
    char trigger_comm[TASK_COMM_LEN];
    // OOM score of killed process
    __s64 score;
    // Total number of pages
    __u64 pages;
    // Tracks if the OOM kill was triggered by a cgroup
    __u32 memcg_oom;
    // OOM score adjustment of killed process
    __s16 score_adj;

    // 2 bytes of padding to force cgroup_name to be 8 byte aligned for bpf_memset
    __s16 pad;

    char cgroup_name[129];
};

#endif /* defined(OOM_KILL_KERN_USER_H) */
