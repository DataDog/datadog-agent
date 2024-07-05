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

BPF_PERF_EVENT_ARRAY_MAP(cuda_events, cuda_event_header_t);

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
int BPF_UPROBE(uprobe_cudaLaunchKernel, const void *func, __u64 grid_xy, __u64 grid_z, __u64 block_xy, __u64 block_z, void **args) {
    cuda_kernel_launch_t launch_data;
    size_t shared_mem = 0;
    size_t *stream_ptr = 0;
    size_t stream;

    bpf_printk("cudaLaunchBefore\n");

    shared_mem = PT_REGS_PARM7(ctx);
    stream_ptr = (size_t *)PT_REGS_PARM8(ctx);

    if (!bpf_probe_read_user(&stream, sizeof(size_t), stream_ptr))
        return 0;

    __builtin_memset(&launch_data, 0, sizeof(launch_data));

    load_dim3(grid_xy, grid_z, &launch_data.grid_size);
    load_dim3(block_xy, block_z, &launch_data.block_size);
    launch_data.header.pid_tgid = bpf_get_current_pid_tgid();
    launch_data.header.ktime_ns = bpf_ktime_get_ns();
    launch_data.header.stream_id = (uint64_t)stream;
    launch_data.header.type = cuda_kernel_launch;
    launch_data.kernel_addr = (uint64_t)func;
    launch_data.shared_mem_size = shared_mem;

    bpf_printk("cudaLaunch\n");

    bpf_ringbuf_output(&cuda_events, &launch_data, sizeof(launch_data), 0);

    return 0;
}

SEC("uprobe/cudaMalloc")
int BPF_UPROBE(uprobe_cudaMalloc, void **devPtr, size_t size) {
    cuda_memory_event_t mem_data;

    bpf_printk("cudaMallocBefore\n");

    __builtin_memset(&mem_data, 0, sizeof(mem_data));

    if (bpf_probe_read_user(&mem_data.addr, sizeof(void *), devPtr))
        return 0;

    mem_data.header.pid_tgid = bpf_get_current_pid_tgid();
    mem_data.header.stream_id = (uint64_t)0;
    mem_data.header.type = cuda_memory_event;
    mem_data.header.ktime_ns = bpf_ktime_get_ns();
    mem_data.size = size;
    mem_data.type = cudaMalloc;

    bpf_printk("cudaMalloc\n");

    bpf_ringbuf_output(&cuda_events, &mem_data, sizeof(mem_data), 0);

    return 0;
}

SEC("uprobe/cudaStreamSynchronize")
int BPF_UPROBE(uprobe_cudaStreamSynchronize, size_t *stream_ptr) {
    // TODO: Send this on return, not on entry
    cuda_sync_t event;
    size_t stream;

    bpf_printk("cudaSyncBefore\n");

    // TODO: Fix why this is failing
    if (!bpf_probe_read_user(&stream, sizeof(size_t), stream_ptr)) stream = 0;

    __builtin_memset(&event, 0, sizeof(event));

    event.header.pid_tgid = bpf_get_current_pid_tgid();
    event.header.stream_id = stream;
    event.header.type = cuda_sync;
    event.header.ktime_ns = bpf_ktime_get_ns();

    bpf_printk("cudaSync\n");

    bpf_ringbuf_output(&cuda_events, &event, sizeof(event), 0);

    return 0;
}
