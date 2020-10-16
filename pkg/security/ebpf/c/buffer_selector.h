#ifndef _BUFFER_SELECTOR_H
#define _BUFFER_SELECTOR_H

#define SYSCALL_MONITOR_KEY 0

struct bpf_map_def SEC("maps/buffer_selector") buffer_selector = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 1,
    .pinning = 0,
    .namespace = "",
};

#endif
