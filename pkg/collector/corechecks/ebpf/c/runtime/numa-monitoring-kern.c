#include "vmlinux.h"
#include "bpf_helpers.h"
#include "map-defs.h"
#include "numa-monitoring-kern-user.h"
#include "bpf_telemetry.h"

#define MAX_NUMA_RUNTIME_ENTRIES 16384

BPF_PERCPU_HASH_MAP(numa_runtime, numa_runtime_key_t, numa_runtime_value_t, MAX_NUMA_RUNTIME_ENTRIES)
BPF_PERCPU_ARRAY(last_switch_ns, __u32, __u64, 1)

SEC("tracepoint/sched/sched_switch")
int tracepoint_numa_sched_switch(void *ctx) {
    __u32 zero = 0;
    __u64 now = bpf_ktime_get_ns();
    __u64 *last = bpf_map_lookup_elem(&last_switch_ns, &zero);
    if (!last) {
        return 0;
    }

    if (*last != 0 && now > *last) {
        numa_runtime_key_t key = {
            .cgroup_id = bpf_get_current_cgroup_id(),
            .numa_node = bpf_get_numa_node_id(),
        };
        numa_runtime_value_t *value = bpf_map_lookup_elem(&numa_runtime, &key);
        if (!value) {
            numa_runtime_value_t initial = {};
            bpf_map_update_with_telemetry(numa_runtime, &key, &initial, BPF_NOEXIST, -EEXIST);
            value = bpf_map_lookup_elem(&numa_runtime, &key);
        }
        if (value) {
            value->runtime_ns += now - *last;
        }
    }
    *last = now;
    return 0;
}

char _license[] SEC("license") = "GPL";
