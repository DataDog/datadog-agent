#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "map-defs.h"
#include "ebpf-kern-user.h"

#define F_DUPFD_CLOEXEC 1030

typedef struct {
    u32 pid;
    int fd;
} map_fd_t;

// *** LRUs are used here because there is often not an appropriate hook to delete entries ***

// cpu+map_id -> len+addr. resize to (num cpus * maximum concurrent maps)
// read from userspace
BPF_LRU_MAP(perf_buffers, perf_buffer_key_t, mmap_region_t, 0)
// map_id -> len+addr for data+consumer. resize to maximum concurrent maps
BPF_LRU_MAP(ring_buffers, u32, ring_mmap_t, 0)

// pid+map_fd -> map_id. resize to maximum concurrent maps
// TODO duplicate FD in perf_buffer_fds may not get deleted
BPF_LRU_MAP(perf_buffer_fds, map_fd_t, u32, 0)
// pid+map_fd -> map_id. resize to maximum concurrent maps
BPF_LRU_MAP(ring_buffer_fds, map_fd_t, u32, 0)

// map_id -> pid. resize to maximum concurrent maps
BPF_LRU_MAP(map_pids, u32, u32, 0)

// TODO max size here may be excessive because entries here are temporary and deleted once inserted into map
// pid+perfevent_fd -> mmap length + address. resize to (num cpus * maximum concurrent maps)
BPF_LRU_MAP(perf_event_mmap, map_fd_t, mmap_region_t, 0)

// *** temporary argument maps ***

// pid_tgid -> struct bpf_map *
BPF_HASH_MAP(bpf_map_new_fd_args, u64, struct bpf_map *, 1)
// perf_event_open args
// pid_tgid -> constant 0
BPF_HASH_MAP(peo_args, u64, u32, 1)

typedef struct {
    // fd used by perf buffer
    int fd;
    // map_id and offset used by ring buffer
    u32 map_id;
    unsigned long offset;
} mmap_args_t;

// pid_tgid -> mmap_args_t
BPF_HASH_MAP(mmap_args, u64, mmap_args_t, 1)
// pid_tgid -> fd+map_id
BPF_HASH_MAP(fcntl_args, u64, int, 1)

// intercept map creation of perf buffers and ring buffers to:
// 1. associate map_fd+pid -> map_id (kernelspace often has map_fd+pid, so store association to allow pivots)
// 2. associate map_id -> pid (needed for userspace to lookup /proc/PID/smaps data)
// 3. initialize map values used to store mmap data (ring buffer only)
SEC("kprobe/security_bpf_map_alloc")
int BPF_KPROBE(k_map_alloc, struct bpf_map *map) {
    enum bpf_map_type mtype = BPF_CORE_READ(map, map_type);
    if (mtype != BPF_MAP_TYPE_PERF_EVENT_ARRAY && mtype != BPF_MAP_TYPE_RINGBUF) {
        return 0;
    }
    u64 pid_tgid = bpf_get_current_pid_tgid();
    log_debug("kprobe/security_bpf_map_alloc: pid_tgid=%llx", pid_tgid);
    bpf_map_update_elem(&bpf_map_new_fd_args, &pid_tgid, &map, BPF_ANY);
    return 0;
}

struct tracepoint_raw_syscalls_sys_exit_t {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    int __syscall_nr;
    long ret;
};

// TODO if we can find a kprobe point further in, that would be preferable for performance reasons
// bpf_map_new_fd doesn't work because ...?
SEC("tracepoint/syscalls/sys_exit_bpf")
int tp_bpf_exit(struct tracepoint_raw_syscalls_sys_exit_t *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct bpf_map **map_ptr = bpf_map_lookup_elem(&bpf_map_new_fd_args, &pid_tgid);
    if (!map_ptr) {
        return 0;
    }

    log_debug("tp/bpf_exit: pid_tgid=%llx", pid_tgid);
    int fd = ctx->ret;
    if (fd <= 0) {
        goto cleanup;
    }

    struct bpf_map *map = *map_ptr;
    u32 map_id = BPF_CORE_READ(map, id);
    enum bpf_map_type mtype = BPF_CORE_READ(map, map_type);

    map_fd_t key = {};
    key.pid = pid_tgid >> 32;
    key.fd = fd;
    log_debug("tp/bpf_exit: map_id=%d fd=%d", map_id, key.fd);
    if (mtype == BPF_MAP_TYPE_PERF_EVENT_ARRAY) {
        // map_fd+pid -> map_id
        bpf_map_update_elem(&perf_buffer_fds, &key, &map_id, BPF_ANY);
        // map_id -> pid
        bpf_map_update_elem(&map_pids, &map_id, &key.pid, BPF_ANY);
    } else if (mtype == BPF_MAP_TYPE_RINGBUF) {
        ring_mmap_t val = {};
        // map_id -> mmap region
        bpf_map_update_elem(&ring_buffers, &map_id, &val, BPF_ANY);
        // map_fd+pid -> map_id
        bpf_map_update_elem(&ring_buffer_fds, &key, &map_id, BPF_ANY);
        // map_id -> pid
        bpf_map_update_elem(&map_pids, &map_id, &key.pid, BPF_ANY);
    }

cleanup:
    bpf_map_delete_elem(&bpf_map_new_fd_args, &pid_tgid);
    return 0;
}



struct tracepoint_syscalls_sys_enter_fcntl_t {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    int __syscall_nr;
    unsigned long fd;
    unsigned long cmd;
    unsigned long arg;
};

// intercept fcntl(2) syscall, because it is used by cilium/ebpf library
// to create duplicated FDs of perf_event FDs. We need to maintain the
// perf_event_fd+pid -> map_id association for all possible FDs.
SEC("tracepoint/syscalls/sys_enter_fcntl")
int tp_fcntl_enter(struct tracepoint_syscalls_sys_enter_fcntl_t *args) {
    // we are only interested if the FD is being duplicated
    if (args->cmd != F_DUPFD_CLOEXEC) {
        return 0;
    }

    // pivot from perf_event_fd+pid -> map_id
    u64 pid_tgid = bpf_get_current_pid_tgid();
    map_fd_t key = {};
    key.pid = pid_tgid >> 32;
    key.fd = args->fd;
    int *map_idp = bpf_map_lookup_elem(&perf_buffer_fds, &key);
    if (!map_idp) {
        return 0;
    }

    int map_id = *map_idp;
    bpf_map_update_elem(&fcntl_args, &pid_tgid, &map_id, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_fcntl")
int tp_fcntl_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    int *map_idp = bpf_map_lookup_elem(&fcntl_args, &pid_tgid);
    if (!map_idp) {
        return 0;
    }
    int map_id = *map_idp;
    if (args->ret <= 0) {
        goto cleanup;
    }

    // store (duplicated) perf_event_fd+pid -> map_id association
    map_fd_t key = {};
    key.pid = pid_tgid >> 32;
    key.fd = (int)args->ret;
    log_debug("sys_exit_fcntl: fd dup new_fd=%d map_id=%d", key.fd, map_id);
    bpf_map_update_elem(&perf_buffer_fds, &key, &map_id, BPF_ANY);

cleanup:
    bpf_map_delete_elem(&fcntl_args, &pid_tgid);
    return 0;
}

// intercept perf_event_open(2) syscall to capture the perf_event_fd values
// we do not know what map or mmap region they correspond to at this point.
SEC("kprobe/security_perf_event_open")
int BPF_KPROBE(k_pe_open, struct perf_event_attr *attr) {
    u32 type = BPF_CORE_READ(attr, type);
    u64 config = BPF_CORE_READ(attr, config);
    u64 sample_type = BPF_CORE_READ(attr, sample_type);

    // only capture perf_event_fds related to perf buffers
    if (type != PERF_TYPE_SOFTWARE ||
        config != PERF_COUNT_SW_BPF_OUTPUT ||
        sample_type != PERF_SAMPLE_RAW) {
        return 0;
    }
    u32 zero = 0;
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&peo_args, &pid_tgid, &zero, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_perf_event_open")
int tp_pe_open_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 *z = bpf_map_lookup_elem(&peo_args, &pid_tgid);
    if (!z) {
        return 0;
    }
    if (args->ret <= 0) {
        goto cleanup;
    }

    // store perf_event_fd+pid -> mmap region (unpopulated at this point)
    mmap_region_t val = {};
    map_fd_t key = {};
    key.fd = (int)args->ret;
    key.pid = pid_tgid >> 32;
    log_debug("tracepoint_sys_exit_perf_event_open: fd=%d", key.fd);
    bpf_map_update_elem(&perf_event_mmap, &key, &val, BPF_ANY);

cleanup:
    bpf_map_delete_elem(&peo_args, &pid_tgid);
    return 0;
}

struct tracepoint_syscalls_sys_enter_mmap_t {
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    int __syscall_nr;
    unsigned long addr;
    unsigned long len;
    unsigned long protection;
    unsigned long flags;
    unsigned long fd;
    unsigned long offset;
};

// capture mmap(2) syscalls to get the actual length and address of mmap-ed regions
SEC("tracepoint/syscalls/sys_enter_mmap")
int tp_mmap_enter(struct tracepoint_syscalls_sys_enter_mmap_t *args) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    mmap_args_t margs = {};

    // perf buffer - perf_event_fd is args->fd
    // pivot from perf_event_fd+pid -> mmap region
    map_fd_t key = {};
    key.fd = (int)args->fd;
    key.pid = pid_tgid >> 32;
    mmap_region_t *val = bpf_map_lookup_elem(&perf_event_mmap, &key);
    if (val) {
        val->len = args->len;
        margs.fd = key.fd;
        bpf_map_update_elem(&mmap_args, &pid_tgid, &margs, BPF_ANY);
        return 0;
    }

    // ring buffer - map_fd is args->fd
    // indexed by map_id, so we must look that up by map_fd+pid
    u32 *map_idp = bpf_map_lookup_elem(&ring_buffer_fds, &key);
    if (!map_idp) {
        return 0;
    }
    ring_mmap_t *ring_val = bpf_map_lookup_elem(&ring_buffers, map_idp);
    if (!ring_val) {
        return 0;
    }
    // choose correct mmap sub-region based on offset
    // offset 0 = consumer sub-region
    // offset x (size of consumer sub-region) = data sub-region
    if (args->offset == 0) {
        ring_val->consumer.len = args->len;
    } else {
        ring_val->data.len = args->len;
    }
    margs.map_id = *map_idp;
    margs.offset = args->offset;
    log_debug("tracepoint_sys_enter_mmap: fd=%d len=%lu", key.fd, args->len);
    bpf_map_update_elem(&mmap_args, &pid_tgid, &margs, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_mmap")
int tp_mmap_exit(struct tracepoint_raw_syscalls_sys_exit_t *args) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    mmap_args_t *margs = bpf_map_lookup_elem(&mmap_args, &pid_tgid);
    if (!margs) {
        return 0;
    }
    if (args->ret <= 0) {
        goto cleanup;
    }

    // lookup mmap region we are dealing with
    // we cannot store this pointer directly in the args map, because we need to write to it
    mmap_region_t *val = NULL;
    if (margs->fd) {
        map_fd_t key = {};
        key.fd = margs->fd;
        key.pid = pid_tgid >> 32;
        val = bpf_map_lookup_elem(&perf_event_mmap, &key);
    } else if (margs->map_id) {
        ring_mmap_t *ring_val = bpf_map_lookup_elem(&ring_buffers, &margs->map_id);
        if (!ring_val) {
            goto cleanup;
        }
        // choose correct sub-region for ring buffer
        if (margs->offset == 0) {
            val = &ring_val->consumer;
        } else {
            val = &ring_val->data;
        }
    }

    if (!val) {
        goto cleanup;
    }
    // store address of mmap region
    val->addr = args->ret;
    log_debug("tracepoint_sys_exit_mmap: len=%lu addr=%lx", val->len, val->addr);

cleanup:
    bpf_map_delete_elem(&mmap_args, &pid_tgid);
    return 0;
}

// perf buffer - array elements are the perf_event fds created earlier and associated with mmap regions
// intercept update, so we can make the correct CPU -> mmap region association
SEC("kprobe/security_bpf")
int BPF_KPROBE(k_map_update, int cmd, union bpf_attr *attr) {
    if (cmd != BPF_MAP_UPDATE_ELEM) {
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();

    // pivot from fd+pid -> map_id
    map_fd_t fdkey = {};
    fdkey.fd = BPF_CORE_READ(attr, map_fd);
    fdkey.pid = pid_tgid >> 32;
    //log_debug("kprobe/map_update_elem: fd=%d", fdkey.fd);
    u32 *map_idp = bpf_map_lookup_elem(&perf_buffer_fds, &fdkey);
    if (!map_idp) {
        return 0;
    }

    // read CPU number from attr syscall argument
    perf_buffer_key_t pb_key = {};
    pb_key.map_id = *map_idp;
    void *cpup = (void*)BPF_CORE_READ(attr, key);
    bpf_probe_read_user(&pb_key.cpu, sizeof(u32), cpup);

    // read perf_event FD from attr syscall argument
    map_fd_t key = {};
    key.pid = pid_tgid >> 32;
    void *fdp = (void*)BPF_CORE_READ(attr, value);
    bpf_probe_read_user(&key.fd, sizeof(int), fdp);

    // pivot from perf_event_fd+pid -> mmap region
    mmap_region_t *infop = bpf_map_lookup_elem(&perf_event_mmap, &key);
    if (infop == NULL) {
        log_debug("kprobe/map_update_elem: no mmap data cpu=%d fd=%d fdptr=%p", pb_key.cpu, key.fd, fdp);
        return 0;
    }

    // make a stack copy of mmap data and store by map_id+cpu, which userspace can know
    mmap_region_t stackinfo = {};
    bpf_probe_read_kernel(&stackinfo, sizeof(mmap_region_t), infop);
    log_debug("map_update_elem: map_id=%d cpu=%d len=%lu", pb_key.map_id, pb_key.cpu, stackinfo.len);
    bpf_map_update_elem(&perf_buffers, &pb_key, &stackinfo, BPF_ANY);
    bpf_map_delete_elem(&perf_event_mmap, &key);
    return 0;
}

char _license[] SEC("license") = "GPL";
