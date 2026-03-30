#ifndef __SOCKET_CONTENTION_KERN_USER_H
#define __SOCKET_CONTENTION_KERN_USER_H

#include "ktypes.h"

struct socket_contention_stats {
    __u64 total_time_ns;
    __u64 min_time_ns;
    __u64 max_time_ns;
    __u32 count;
    __u32 flags;
};

#endif
