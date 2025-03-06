#ifndef DISCOVERY_TYPES
#define DISCOVERY_TYPES

#include "ktypes.h"

struct network_stats_key {
    __u32 pid;
};

struct network_stats {
    __u64 rx;
    __u64 tx;
};

#endif /* defined(DISCOVERY_TYPES) */
