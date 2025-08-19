#include "ktypes.h"
#include "bpf_metadata.h"
#include "compiler.h"
#include "map-defs.h"
#include "bpf_tracing.h"
#include "bpf_telemetry.h"

BPF_HASH_MAP(error_map, u32, u32, 2);
BPF_HASH_MAP(suppress_map, u32, u32, 2);
BPF_HASH_MAP(shared_map, u32, u32, 1);

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

    u32 j = 1;
    u32* val = bpf_map_lookup_elem(&shared_map, &j);
    if (val == NULL) {
        bpf_map_update_with_telemetry(shared_map, &j, &j, BPF_ANY);
        j++;

        bpf_map_update_with_telemetry(shared_map, &j, &j, BPF_ANY);
    }

    return 0;
}

static int __always_inline is_telemetry_call(struct pt_regs *ctx) {
    u32 cmd = PT_REGS_PARM3(ctx);
    return cmd == 0xfafadead;
};

SEC("kprobe/do_vfs_ioctl")
int kprobe__do_vfs_ioctl(struct pt_regs *ctx) {
    if (!is_telemetry_call(ctx)) {
        return 0;
    }

    // we must start updating from a value we know does not exist in the map already
    // from the call to `kprobe__vs_open`
    u32 i = 0xabcd;
    bpf_map_update_with_telemetry(shared_map, &i, &i, BPF_ANY);
    i++;
    bpf_map_update_with_telemetry(shared_map, &i, &i, BPF_ANY);
    i++;
    bpf_map_update_with_telemetry(shared_map, &i, &i, BPF_ANY);

    // 2 E2BIG errors

    return 0;
}

char _license[] SEC("license") = "GPL";
