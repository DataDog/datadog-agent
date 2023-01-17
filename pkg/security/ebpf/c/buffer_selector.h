#ifndef _BUFFER_SELECTOR_H
#define _BUFFER_SELECTOR_H

#define ERPC_MONITOR_KEY        1
#define DISCARDER_MONITOR_KEY   2

struct bpf_map_def SEC("maps/buffer_selector") buffer_selector = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 3,
};

static __attribute__((always_inline))
struct bpf_map_def *select_buffer(struct bpf_map_def *front_buffer,
                                  struct bpf_map_def *back_buffer,
                                  u32 selector_key) {
    u32 *buffer_id = bpf_map_lookup_elem(&buffer_selector, &selector_key);
    if (buffer_id == NULL) {
        return NULL;
    }

    return *buffer_id ? back_buffer : front_buffer;
}

#endif
