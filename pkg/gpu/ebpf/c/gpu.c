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
BPF_PERF_EVENT_ARRAY_MAP(cuda_memory_events, cuda_memory_event_t);

// cudaLaunchKernel receives the dim3 argument by value, which gets translated as
// a 64 bit register with the x and y values in the lower and upper 32 bits respectively,
// and the z value in a separate register. This function decodes those values into a dim3 struct.
static inline void load_dim3(__u64 xy, __u64 z, dim3 *dst) {
    __u64 mask = 0xffffffff;
    dst->x = xy & mask;
    dst->y = xy >> 32;
    dst->z = z;
}

SEC("uprobe/cudaLaunchKernel")
int BPF_UPROBE(uprobe_cudaLaunchKernel, const void *func, __u64 grid_xy, __u64 grid_z, __u64 block_xy, __u64 block_z, void **args, size_t sharedMem, void *stream) {
    uint64_t probe_id = 0;
    cuda_kernel_launch_t launch_data;
    LOAD_CONSTANT("probe_id", probe_id);

    __builtin_memset(&launch_data, 0, sizeof(launch_data));

    load_dim3(grid_xy, grid_z, &launch_data.grid_size);
    load_dim3(block_xy, block_z, &launch_data.block_size);
    launch_data.pid_tgid = bpf_get_current_pid_tgid();
    launch_data.probe_id = probe_id;
    launch_data.kernel_addr = (uint64_t)func;
    launch_data.ktime_ns = bpf_ktime_get_ns();
    launch_data.stream_id = (uint64_t)stream;
    launch_data.shared_mem_size = sharedMem;

    bpf_ringbuf_output(&cuda_kernel_launches, &launch_data, sizeof(launch_data), 0);

    return 0;
}

SEC("uprobe/cudaMalloc")
int BPF_UPROBE(uprobe_cudaMalloc, void **devPtr, size_t size) {
    uint64_t probe_id = 0;
    cuda_memory_event_t mem_data;
    LOAD_CONSTANT("probe_id", probe_id);

    __builtin_memset(&mem_data, 0, sizeof(mem_data));

    if (bpf_probe_read_user(&mem_data.addr, sizeof(void *), devPtr))
        return 0;

    mem_data.pid_tgid = bpf_get_current_pid_tgid();
    mem_data.probe_id = probe_id;
    mem_data.size = size;
    mem_data.type = cudaMalloc;

    bpf_ringbuf_output(&cuda_memory_events, &mem_data, sizeof(mem_data), 0);

    return 0;
}
