#include "vmlinux_arm64.h"
#include "bpf_helpers.h"
 
struct bpf_map_def SEC("maps") events = {
    .type        = BPF_MAP_TYPE_RINGBUF,
    .max_entries = 1<<24,
};
