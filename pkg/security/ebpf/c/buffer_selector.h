#ifndef _BUFFER_SELECTOR_H
#define _BUFFER_SELECTOR_H

#define SYSCALL_MONITOR_KEY 0
#define ERPC_MONITOR_KEY    1

struct bpf_map_def SEC("maps/buffer_selector") buffer_selector = {
    .type = BPF_MAP_TYPE_ARRAY,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 2,
    .pinning = 0,
    .namespace = "",
};

static __attribute__((always_inline))
struct bpf_map_def *select_buffer(struct bpf_map_def *front_buffer,
                                  struct bpf_map_def *back_buffer,
                                  u32 selector_key) {
    u32 *buffer_id = bpf_map_lookup_elem(&buffer_selector, &selector_key);
    if (buffer_id == NULL)
        return NULL;

    return *buffer_id ? back_buffer : front_buffer;
}

static __attribute__((always_inline))
void *bpf_map_lookup_or_try_init(struct bpf_map_def *map, void *key, void *zero) {
    if (map == NULL) {
        return NULL;
    }

    void *value = bpf_map_lookup_elem(map, key);
    if (value != NULL)
        return value;

    // Use BPF_NOEXIST to prevent race condition
    if (bpf_map_update_elem(map, key, zero, BPF_NOEXIST) < 0)
        return NULL;

    return bpf_map_lookup_elem(map, key);
}

#endif
