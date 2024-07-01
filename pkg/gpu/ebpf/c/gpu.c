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
int BPF_UPROBE(uprobe_cudaLaunchKernel, const void *func, dim3 *gridDim, dim3 *blockDim, void **args, size_t sharedMem, void *stream) {
    uint64_t probe_id = 0;
    cuda_kernel_launch_t launch_data;
    LOAD_CONSTANT("probe_id", probe_id);

    __builtin_memset(&launch_data, 0, sizeof(launch_data));

    if (bpf_probe_read_user(&launch_data.grid_size, sizeof(dim3), gridDim))
        return 0;

    if (bpf_probe_read_user(&launch_data.block_size, sizeof(dim3), blockDim))
        return 0;

    launch_data.pid_tgid = bpf_get_current_pid_tgid();
    launch_data.probe_id = probe_id;
    launch_data.kernel_addr = (uint64_t)func;
    launch_data.ktime_ns = bpf_ktime_get_ns();
    launch_data.stream_id = (uint64_t)stream;
    launch_data.shared_mem_size = sharedMem;

    bpf_ringbuf_output(&cuda_kernel_launches, &launch_data, sizeof(launch_data), 0);

    return 0;
}
