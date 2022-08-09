#ifndef PID_MAPPER_H
#define PID_MAPPER_H
#include "bpf_helpers.h"

struct bpf_map_def SEC("maps/sock_to_pid") sock_to_pid = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(__u64),
    .value_size = sizeof(__u32),
    .max_entries = 0,
    .pinning = 0,
    .namespace = "",
};

#endif 
