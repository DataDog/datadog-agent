#ifndef TCP_QUEUE_LENGTH_KERN_USER_H
#define TCP_QUEUE_LENGTH_KERN_USER_H

#include "ktypes.h"

struct stats_key {
    char cgroup[129];
};

struct stats_value {
    __u32 read_buffer_max_usage;
    __u32 write_buffer_max_usage;
};

#endif /* defined(TCP_QUEUE_LENGTH_KERN_USER_H) */
