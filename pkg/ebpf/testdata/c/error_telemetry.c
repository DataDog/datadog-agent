#include "ktypes.h"
#include "bpf_metadata.h"
#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#endif
#include "compiler.h"
#include "map-defs.h"
#include "bpf_tracing.h"
#include "bpf_telemetry.h"

BPF_HASH_MAP(testmap1, u32, u32, 2);
BPF_HASH_MAP(testmap2, u32, u32, 2);

#define E2BIG 16

SEC("kprobe/vfs_open")
int kprobe__vfs_open(void *ctx) {
    u32 i = 0;
    i++;
    bpf_map_update_with_telemetry(testmap1, &i, &i, BPF_ANY);
    i++;
    bpf_map_update_with_telemetry(testmap1, &i, &i, BPF_ANY);
    i++;
    bpf_map_update_with_telemetry(testmap1, &i, &i, BPF_ANY);

//#pragma unroll
//    for (int i = 0; i < 10; i++) {
//        bpf_map_update_with_telemetry(testmap2, &i, &i, BPF_ANY, -E2BIG);
//    }

    return 0;
}
