#ifndef _FILTERS_H
#define _FILTERS_H

#include "process.h"

enum policy_mode
{
    NO_FILTER = 0,
    ACCEPT = 1,
    DENY = 2,
};

enum policy_flags
{
    BASENAME = 1,
    FLAGS = 2,
    MODE = 4,
    PARENT_NAME = 8,
};

struct policy_t {
    char mode;
    char flags;
};

struct bpf_map_def SEC("maps/filter_policy") filter_policy = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(struct policy_t),
    .max_entries = EVENT_MAX,
    .pinning = 0,
    .namespace = "",
};

#endif
