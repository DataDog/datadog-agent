#include "ktypes.h"
#include "bpf_metadata.h"
#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#endif
#include "compiler.h"
#include "map-defs.h"
#include "bpf_tracing.h"
#include "bpf_telemetry.h"

BPF_HASH_MAP(error_map, u32, u32, 2);
BPF_HASH_MAP(suppress_map, u32, u32, 2);

#define E2BIG 7

SEC("kprobe/vfs_open")
int kprobe__vfs_open(int *ctx) {
    u32 i = 0;
    bpf_map_update_with_telemetry(error_map, &i, &i, BPF_ANY);
    i++;
    bpf_map_update_with_telemetry(error_map, &i, &i, BPF_ANY);
    i++;
    bpf_map_update_with_telemetry(error_map, &i, &i, BPF_ANY);

    bpf_map_update_with_telemetry(suppress_map, &i, &i, BPF_ANY, -E2BIG);
    i++;
    bpf_map_update_with_telemetry(suppress_map, &i, &i, BPF_ANY, -E2BIG);
    i++;
    bpf_map_update_with_telemetry(suppress_map, &i, &i, BPF_ANY, -E2BIG);

    char buf[16];
    bpf_probe_read_with_telemetry(&buf, 16, (void *)0xdeadbeef);

    return 0;
}

char _license[] SEC("license") = "GPL";
