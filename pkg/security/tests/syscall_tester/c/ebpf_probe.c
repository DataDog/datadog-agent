#include "bpf_helpers.h"

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

struct bpf_map_def SEC("maps/cache") cache = {
    .type = BPF_MAP_TYPE_HASH,
    .key_size = sizeof(u32),
    .value_size = sizeof(u32),
    .max_entries = 10,
};

#pragma clang diagnostic pop

SEC("kprobe/vfs_open")
int kprobe_vfs_open(void *ctx) {
    u32 key = 1;
    u32 *value = bpf_map_lookup_elem(&cache, &key);
    if (value == 0) {
        bpf_printk("map entry 1 is empty!\n");
    }
    bpf_printk("hello world!\n");
    return 0;
}

char _license[] SEC("license") = "GPL";
__u32 _version SEC("version") = 0xFFFFFFFE;
