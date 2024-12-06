#define BPF_NO_PRESERVE_ACCESS_INDEX
#define BPF_NO_GLOBAL_DATA

#ifdef COMPILE_RUNTIME
#include "kconfig.h"
#include <linux/ptrace.h>
#endif

#include "ktypes.h"
#include "bpf_metadata.h"
#include "bpf_helpers.h"
#include "bpf_helpers_custom.h"
#include "bpf_tracing.h"
#include "compiler.h"
#include "map-defs.h"
#include "bpf_telemetry.h"
#include "bpf_builtins.h"
#include "cgroup.h"

#include "types.h"

BPF_RINGBUF_MAP(cuda_events, cuda_event_header_t);
BPF_LRU_MAP(cuda_alloc_cache, __u64, cuda_alloc_request_args_t, 1024)
BPF_LRU_MAP(cuda_sync_cache, __u64, __u64, 1024)
BPF_LRU_MAP(cuda_set_device_cache, __u64, int, 1024)

// cudaLaunchKernel receives the dim3 argument by value, which gets translated as
// a 64 bit register with the x and y values in the lower and upper 32 bits respectively,
// and the z value in a separate register. This function decodes those values into a dim3 struct.
static inline void load_dim3(__u64 xy, __u64 z, dim3 *dst) {
    __u64 mask = 0xffffffff;
    dst->x = xy & mask;
    dst->y = xy >> 32;
    dst->z = z;
}

static inline void fill_header(cuda_event_header_t *header, __u64 stream_id, cuda_event_type_t type) {
    header->pid_tgid = bpf_get_current_pid_tgid();
    header->ktime_ns = bpf_ktime_get_ns();
    header->stream_id = stream_id;
    header->type = type;
    get_cgroup_name(header->cgroup, sizeof(header->cgroup));
}

SEC("uprobe/cudaLaunchKernel")
int BPF_UPROBE(uprobe__cudaLaunchKernel, const void *func, __u64 grid_xy, __u64 grid_z, __u64 block_xy, __u64 block_z, void **args) {
    cuda_kernel_launch_t launch_data = { 0 };
    long read_ret = 0;
    __u64 shared_mem = 0;
    __u64 stream = 0;

    shared_mem = PT_REGS_USER_PARM7(ctx, read_ret);
    if (read_ret < 0) {
        log_debug("cudaLaunchKernel: failed to read shared_mem");
        return 0;
    }

    stream = PT_REGS_USER_PARM8(ctx, read_ret);
    if (read_ret < 0) {
        log_debug("cudaLaunchKernel: failed to read stream");
        return 0;
    }

    load_dim3(grid_xy, grid_z, &launch_data.grid_size);
    load_dim3(block_xy, block_z, &launch_data.block_size);
    fill_header(&launch_data.header, stream, cuda_kernel_launch);
    launch_data.kernel_addr = (uint64_t)func;
    launch_data.shared_mem_size = shared_mem;

    log_debug("cudaLaunchKernel: EMIT[1/2] pid_tgid=%llu, ts=%llu", launch_data.header.pid_tgid, launch_data.header.ktime_ns);
    log_debug("cudaLaunchKernel: EMIT[2/2] kernel_addr=0x%llx, shared_mem=%llu, stream_id=%llu", launch_data.kernel_addr, launch_data.shared_mem_size, launch_data.header.stream_id);

    bpf_ringbuf_output_with_telemetry(&cuda_events, &launch_data, sizeof(launch_data), 0);

    return 0;
}

SEC("uprobe/cudaMalloc")
int BPF_UPROBE(uprobe__cudaMalloc, void **devPtr, size_t size) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    cuda_alloc_request_args_t args = { .devPtr = devPtr, .size = size };

    log_debug("cudaMalloc: pid=%llu, devPtr=%llx, size=%lu", pid_tgid, (__u64)devPtr, size);
    bpf_map_update_with_telemetry(cuda_alloc_cache, &pid_tgid, &args, BPF_ANY);

    return 0;
}

SEC("uretprobe/cudaMalloc")
int BPF_URETPROBE(uretprobe__cudaMalloc) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    cuda_alloc_request_args_t *args;
    cuda_memory_event_t mem_data = { 0 };

    log_debug("cudaMalloc[ret]: pid=%llx\n", pid_tgid);

    args = bpf_map_lookup_elem(&cuda_alloc_cache, &pid_tgid);
    if (!args) {
        log_debug("cudaMalloc[ret]: failed to find cudaMalloc request");
        return 0;
    }

    fill_header(&mem_data.header, 0, cuda_memory_event);
    mem_data.type = cudaMalloc;
    mem_data.size = args->size;

    if (bpf_probe_read_user_with_telemetry(&mem_data.addr, sizeof(void *), args->devPtr)) {
        log_debug("cudaMalloc[ret]: failed to read devPtr from cudaMalloc at 0x%llx", (__u64)args->devPtr);
        goto out;
    }

    log_debug("cudaMalloc[ret]: EMIT size=%llu, addr=0x%llx, ts=%llu", mem_data.size, (__u64)mem_data.addr, mem_data.header.ktime_ns);

    bpf_ringbuf_output_with_telemetry(&cuda_events, &mem_data, sizeof(mem_data), 0);

out:
    bpf_map_delete_elem(&cuda_alloc_cache, &pid_tgid);
    return 0;
}

SEC("uprobe/cudaFree")
int BPF_UPROBE(uprobe__cudaFree, void *mem) {
    cuda_memory_event_t mem_data = { 0 };

    fill_header(&mem_data.header, 0, cuda_memory_event);
    mem_data.size = 0;
    mem_data.addr = (uint64_t)mem;
    mem_data.type = cudaFree;

    bpf_ringbuf_output_with_telemetry(&cuda_events, &mem_data, sizeof(mem_data), 0);

    return 0;
}

SEC("uprobe/cudaStreamSynchronize")
int BPF_UPROBE(uprobe__cudaStreamSynchronize, __u64 stream) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();

    log_debug("cudaStreamSynchronize: pid=%llu, stream=%llu", pid_tgid, stream);
    bpf_map_update_with_telemetry(cuda_sync_cache, &pid_tgid, &stream, BPF_ANY);

    return 0;
}

SEC("uretprobe/cudaStreamSynchronize")
int BPF_URETPROBE(uretprobe__cudaStreamSynchronize) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *stream = NULL;
    cuda_sync_t event = { 0 };

    log_debug("cudaStreamSyncronize[ret]: pid=%llx\n", pid_tgid);

    stream = bpf_map_lookup_elem(&cuda_sync_cache, &pid_tgid);
    if (!stream) {
        log_debug("cudaStreamSyncronize[ret]: failed to find cudaStreamSyncronize request");
        return 0;
    }

    fill_header(&event.header, *stream, cuda_sync);

    log_debug("cudaStreamSynchronize[ret]: EMIT cudaSync pid_tgid=%llu, stream_id=%llu", event.header.pid_tgid, event.header.stream_id);

    bpf_ringbuf_output_with_telemetry(&cuda_events, &event, sizeof(event), 0);
    bpf_map_delete_elem(&cuda_sync_cache, &pid_tgid);

    return 0;
}

SEC("uprobe/cudaSetDevice")
int BPF_UPROBE(uprobe__cudaSetDevice, int device) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();

    log_debug("cudaSetDevice: pid_tgid=%llu, device=%u", pid_tgid, device);
    bpf_map_update_with_telemetry(cuda_set_device_cache, &pid_tgid, &device, BPF_ANY);

    return 0;
}

SEC("uretprobe/cudaSetDevice")
int BPF_URETPROBE(uretprobe__cudaSetDevice) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    int *device = NULL;
    cuda_set_device_event_t event = { 0 };
    __u32 retval = PT_REGS_RC(ctx);

    log_debug("cudaSetDevice[ret]: pid_tgid=%llu, retval=%d\n", pid_tgid, retval);

    if (retval != 0) {
        // Do not emit event if cudaSetDevice failed
        goto cleanup;
    }

    device = bpf_map_lookup_elem(&cuda_set_device_cache, &pid_tgid);
    if (!device) {
        log_debug("cudaSetDevice[ret]: failed to find cudaSetDevice request");
        return 0;
    }

    fill_header(&event.header, 0, cuda_set_device);
    event.device = *device;

    log_debug("cudaSetDevice: EMIT pid_tgid=%llu, device=%d", event.header.pid_tgid, *device);
    bpf_ringbuf_output_with_telemetry(&cuda_events, &event, sizeof(event), 0);

cleanup:
    bpf_map_delete_elem(&cuda_sync_cache, &pid_tgid);

    return 0;
}

char __license[] SEC("license") = "GPL";
