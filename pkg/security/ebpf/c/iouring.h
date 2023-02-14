#ifndef _IOURING_H_
#define _IOURING_H_

struct bpf_map_def SEC("maps/io_uring_ctx_pid") io_uring_ctx_pid = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(void*),
    .value_size = sizeof(u64),
    .max_entries = 2048,
};

void __attribute__((always_inline)) cache_ioctx_pid_tgid(void *ioctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
#ifdef DEBUG
    bpf_printk("pid = %d", (u32)pid_tgid);
    bpf_printk("tgid = %d", pid_tgid >> 32);
    bpf_printk("ioctx in = %p", ioctx);
#endif
    bpf_map_update_elem(&io_uring_ctx_pid, &ioctx, &pid_tgid, BPF_ANY);
}

struct tracepoint_io_uring_io_uring_create_t
{
    unsigned short common_type;
    unsigned char common_flags;
    unsigned char common_preempt_count;
    int common_pid;

    int fd;
	void *ctx;
	u32 sq_entries;
	u32 cq_entries;
	u32 flags;
};

SEC("tracepoint/io_uring/io_uring_create")
int io_uring_create(struct tracepoint_io_uring_io_uring_create_t *args) {
    void *ioctx = args->ctx;
    cache_ioctx_pid_tgid(ioctx);
    return 0;
}

SEC("kretprobe/io_ring_ctx_alloc")
int kretprobe_io_ring_ctx_alloc(struct pt_regs *ctx) {
    void *ioctx = (void *)PT_REGS_RC(ctx);
    cache_ioctx_pid_tgid(ioctx);
    return 0;
}

SEC("kprobe/io_allocate_scq_urings")
int kprobe_io_allocate_scq_urings(struct pt_regs *ctx) {
    void *ioctx = (void *)PT_REGS_PARM1(ctx);
    cache_ioctx_pid_tgid(ioctx);
    return 0;
}

SEC("kprobe/io_sq_offload_start")
int kprobe_io_sq_offload_start(struct pt_regs *ctx) {
    void *ioctx = (void *)PT_REGS_PARM1(ctx);
    cache_ioctx_pid_tgid(ioctx);
    return 0;
}

u64 __attribute__((always_inline)) get_pid_tgid_from_iouring(void *req) {
    u64 ioctx_offset;
    LOAD_CONSTANT("iokiocb_ctx_offset", ioctx_offset);

    void *ioctx;
    int ret = bpf_probe_read(&ioctx, sizeof(void*), req + ioctx_offset);
    if (ret < 0) {
        return 0;
    }

#ifdef DEBUG
    bpf_printk("ioctx out = %p", ioctx);
#endif

    u64 *pid_tgid_ptr = bpf_map_lookup_elem(&io_uring_ctx_pid, &ioctx);
    if (pid_tgid_ptr) {
        return *pid_tgid_ptr;
    } else {
        return 0;
    }
}

#endif
