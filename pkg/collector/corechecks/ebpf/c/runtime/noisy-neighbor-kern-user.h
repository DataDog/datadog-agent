#ifndef __NOISY_NEIGHBOR_KERN_USER_H
#define __NOISY_NEIGHBOR_KERN_USER_H

#include "ktypes.h"

typedef struct {
    __u64 prev_cgroup_id;
    __u64 cgroup_id;
    __u64 runq_lat;
    __u64 ts;
    __u64 pid;
    __u64 prev_pid;

    char prev_cgroup_name[129];
    char cgroup_name[129];
} runq_event_t;

#endif
