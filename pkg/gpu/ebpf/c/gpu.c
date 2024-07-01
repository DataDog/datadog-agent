#define BPF_NO_PRESERVE_ACCESS_INDEX
#define BPF_NO_GLOBAL_DATA

#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"
#include "bpf_tracing.h"
#include "compiler.h"
#include "map-defs.h"

#include "types.h"

char __license[] SEC("license") = "GPL";

BPF_PERF_EVENT_ARRAY_MAP(cuda_kernel_launches, cuda_kernel_launch_t);

SEC("uprobe/cudaLaunchKernel")
int uprobe_cudaLaunchKernel(struct pt_regs *ctx) {
    uint64_t probe_id = 0;
    cuda_kernel_launch_t launch_data;
    LOAD_CONSTANT("probe_id", probe_id);

    __builtin_memset(&launch_data, 0, sizeof(launch_data));

    struct dim3 grid_dim, block_dim;

    if (bpf_probe_read_user(&grid_dim, sizeof(grid_dim), (void *)PT_REGS_PARM2(ctx)))
        return 0;

    if (bpf_probe_read_user(&block_dim, sizeof(block_dim), (void *)PT_REGS_PARM3(ctx)))
        return 0;

    uint64_t grid_size = grid_dim.x * grid_dim.y * grid_dim.z;
    uint64_t block_size = block_dim.x * block_dim.y * block_dim.z;

    launch_data.pid_tgid = bpf_get_current_pid_tgid(),
    launch_data.probe_id = probe_id,
    launch_data.kernel_addr = PT_REGS_PARM1(ctx),
    launch_data.ktime_ns = bpf_ktime_get_ns(),
    launch_data.grid_size = grid_size,
    launch_data.block_size = block_size,
    launch_data.stream_id = PT_REGS_PARM6(ctx),
    launch_data.shared_mem_size = (size_t)PT_REGS_PARM5(ctx),

    bpf_ringbuf_output(&cuda_kernel_launches, &launch_data, sizeof(launch_data), 0);

    return 0;
}
