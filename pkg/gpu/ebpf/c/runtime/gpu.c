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
#include "pid_tgid.h"

#include "types.h"

BPF_RINGBUF_MAP(cuda_events, cuda_event_header_t);
BPF_LRU_MAP(cuda_alloc_cache, __u64, cuda_alloc_request_args_t, 1024)
BPF_LRU_MAP(cuda_sync_cache, __u64, __u64, 1024)
BPF_LRU_MAP(cuda_set_device_cache, __u64, int, 1024)
BPF_LRU_MAP(cuda_event_query_cache, __u64, __u64, 1024) // maps PID/TGID -> event
BPF_LRU_MAP(cuda_memcpy_cache, __u64, __u64, 1024) // maps PID/TGID -> stream
BPF_HASH_MAP(cuda_event_to_stream, cuda_event_key_t, cuda_event_value_t, 1024) // maps PID + event -> stream id

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

SEC("uprobe/cudaEventRecord")
int BPF_UPROBE(uprobe__cudaEventRecord, __u64 event, __u64 stream) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    cuda_event_key_t key = { 0 };
    cuda_event_value_t value = { 0 };

    key.event = event;
    key.pid = GET_USER_MODE_PID(pid_tgid);

    value.stream = stream;
    value.last_access_ktime_ns = bpf_ktime_get_ns();

    log_debug("cudaEventRecord: pid_tgid=%llu, event=%llu, stream=%llu", pid_tgid, event, stream);

    // Add the event regardless of return value to avoid having an extra retprobe. If
    // the call fails, the map cleaner will clean it up.
    bpf_map_update_with_telemetry(cuda_event_to_stream, &key, &value, BPF_ANY);

    return 0;
}

SEC("uprobe/cudaEventQuery")
int BPF_UPROBE(uprobe__cudaEventQuery, __u64 event) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();

    log_debug("cudaEventQuery: pid_tgid=%llu, event=%llu", pid_tgid, event);
    bpf_map_update_with_telemetry(cuda_event_query_cache, &pid_tgid, &event, BPF_ANY);

    return 0;
}

SEC("uprobe/cudaEventSynchronize")
int BPF_UPROBE(uprobe__cudaEventSynchronize, __u64 event) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();

    log_debug("cudaEventSynchronize: pid_tgid=%llu, event=%llu", pid_tgid, event);
    bpf_map_update_with_telemetry(cuda_event_query_cache, &pid_tgid, &event, BPF_ANY);

    return 0;
}

static inline int _event_api_trigger_sync(__u32 retval, void *event_cache_map) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *event = NULL;
    cuda_event_key_t event_key = { 0 };
    cuda_event_value_t *event_value = NULL;
    cuda_sync_t sync_event = { 0 };

    if (retval != 0) {
        // Do not emit event if the function failed
        goto cleanup;
    }

    event = bpf_map_lookup_elem(event_cache_map, &pid_tgid);
    if (!event) {
        goto cleanup;
    }

    log_debug("cudaEventQuery/Synchronize[ret]: pid_tgid=%llu -> event = %llu", pid_tgid, *event);

    event_key.event = *event;
    event_key.pid = GET_USER_MODE_PID(pid_tgid);
    event_value = bpf_map_lookup_elem(&cuda_event_to_stream, &event_key);
    if (!event_value) {
        goto cleanup;
    }

    event_value->last_access_ktime_ns = bpf_ktime_get_ns();
    log_debug("cudaEventQuery/Synchronize[ret]: pid_tgid=%llu -> event = %llu -> stream = %llu", pid_tgid, *event, event_value->stream);

    fill_header(&sync_event.header, event_value->stream, cuda_sync);

    bpf_ringbuf_output_with_telemetry(&cuda_events, &sync_event, sizeof(sync_event), 0);

cleanup:
    // We don't remove the event from the stream map here, as it can be queried multiple times
    // Only remove it on cudaEventDestroy.
    // In this function we only remove the pid/tgid entry in the event cache that helps us link
    // the pid/tgid to the event.
    bpf_map_delete_elem(event_cache_map, &pid_tgid);

    return 0;
}

SEC("uretprobe/cudaEventQuery")
int BPF_URETPROBE(uretprobe__cudaEventQuery) {
    __u64 retval = PT_REGS_RC(ctx);

    log_debug("cudaEventQuery[ret]: pid_tgid=%llu, retval=%llu", bpf_get_current_pid_tgid(), retval);
    return _event_api_trigger_sync(retval, &cuda_event_query_cache);
}

SEC("uretprobe/cudaEventSynchronize")
int BPF_URETPROBE(uretprobe__cudaEventSynchronize) {
    __u64 retval = PT_REGS_RC(ctx);

    log_debug("cudaEventSynchronize[ret]: pid_tgid=%llu, retval=%llu", bpf_get_current_pid_tgid(), retval);
    return _event_api_trigger_sync(retval, &cuda_event_query_cache);
}

SEC("uprobe/cudaEventDestroy")
int BPF_UPROBE(uprobe__cudaEventDestroy, __u64 event) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    cuda_event_key_t key = { 0 };

    key.event = event;
    key.pid = GET_USER_MODE_PID(pid_tgid);

    log_debug("cudaEventDestroy: pid_tgid=%llu, event=%llu", pid_tgid, event);

    // If this deletion doesn't get triggered, the map cleaner will clean these entries up
    bpf_map_delete_elem(&cuda_event_to_stream, &key);

    return 0;
}

SEC("uprobe/cudaMemcpy")
int BPF_UPROBE(uprobe__cudaMemcpy, void *dst, const void *src, size_t count, int kind) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();

    log_debug("cudaMemcpy: pid_tgid=%llu", pid_tgid);
    bpf_map_update_with_telemetry(cuda_memcpy_cache, &pid_tgid, &count, BPF_ANY);

    return 0;
}

SEC("uretprobe/cudaMemcpy")
int BPF_URETPROBE(uretprobe__cudaMemcpy) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *count = NULL;
    cuda_sync_t event = { 0 };

    log_debug("cudaMemcpy[ret]: pid_tgid=%llu\n", pid_tgid);

    count = bpf_map_lookup_elem(&cuda_memcpy_cache, &pid_tgid);
    if (!count) {
        log_debug("cudaMemcpy[ret]: failed to find cudaMemcpy request");
        return 0;
    }

    fill_header(&event.header, 0, cuda_sync);

    // According to https://docs.nvidia.com/cuda/cuda-c-programming-guide/index.html#concurrent-execution-between-host-and-device
    // most memory transfers force a synchronization on the global stream. Note that other streams might or might not sync,
    // but for now we don't have fine-grained synchronization data for streams.

    log_debug("cudaMemcpy[ret]: EMIT cudaSync pid_tgid=%llu", event.header.pid_tgid);

    bpf_ringbuf_output_with_telemetry(&cuda_events, &event, sizeof(event), 0);
    bpf_map_delete_elem(&cuda_memcpy_cache, &pid_tgid);

    return 0;
}

SEC("uprobe/setenv")
int BPF_UPROBE(uprobe__setenv, const char *name, const char *value, int overwrite) {
    // Check if the env var is CUDA_VISIBLE_DEVICES. This is BPF_UPROBE, so we can't use a string
    // comparison.
    const char cuda_visible_devices[] = "CUDA_VISIBLE_DEVICES";
    char name_buf[sizeof(cuda_visible_devices)];

    // bpf_probe_read_user_str is available from kernel 5.5, our minimum kernel version is 5.8.0
    int res = bpf_probe_read_user_str_with_telemetry(name_buf, sizeof(name_buf), name);
    if (res < 0) {
        return 0;
    }

    // return value of bpf_probe_read_user_str_with_telemetry is the length of the string read,
    // including the NULL byte. If the string is not the same length, it's not CUDA_VISIBLE_DEVICES.
    if (res != sizeof(cuda_visible_devices)) {
        return 0;
    }

    // bpf_strncmp is available in kernel 5.17, our minimum kernel version is 5.8.0
    // so we need to do a manual comparison
    for (int i = 0; i < sizeof(cuda_visible_devices); i++) {
        if (name_buf[i] != cuda_visible_devices[i]) {
            return 0;
        }
    }

    cuda_visible_devices_set_t event = { 0 };

    if (bpf_probe_read_user_str_with_telemetry(event.visible_devices, sizeof(event.visible_devices), value) < 0) {
        return 0;
    }

    fill_header(&event.header, 0, cuda_visible_devices_set);

    bpf_ringbuf_output_with_telemetry(&cuda_events, &event, sizeof(event), 0);
    return 0;
}

char __license[] SEC("license") = "GPL";
