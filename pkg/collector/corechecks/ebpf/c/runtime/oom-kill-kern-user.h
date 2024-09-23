#ifndef OOM_KILL_KERN_USER_H
#define OOM_KILL_KERN_USER_H

#include "ktypes.h"

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif

struct oom_stats {
    char cgroup_name[129];
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
    // OOM score adjustment of killed process
    __s16 score_adj;
    // Total number of pages
    __u64 pages;
    // Tracks if the OOM kill was triggered by a cgroup
    __u32 memcg_oom;
};

#endif /* defined(OOM_KILL_KERN_USER_H) */
