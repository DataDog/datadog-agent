#ifndef _IOURING_H_
#define _IOURING_H_

struct bpf_map_def SEC("maps/io_uring_ctx_pid") io_uring_ctx_pid = {
    .type = BPF_MAP_TYPE_LRU_HASH,
    .key_size = sizeof(void*),
    .value_size = sizeof(u64),
    .max_entries = 2048,
    .pinning = 0,
    .namespace = "",
};

SEC("kretprobe/io_ring_ctx_alloc")
int kretprobe_io_ring_ctx_alloc(struct pt_regs *ctx) {
    void *ioctx = (void *)PT_REGS_RC(ctx);
    u64 pid_tgid = bpf_get_current_pid_tgid();
    bpf_map_update_elem(&io_uring_ctx_pid, &ioctx, &pid_tgid, BPF_ANY);
    return 0;
}

u64 __attribute__((always_inline)) get_pid_tgid_from_iouring(void *req) {
    void *ioctx;
    bpf_probe_read(&ioctx, sizeof(void*), req + 80); // TODO constantify
    u64 *pid_tgid_ptr = bpf_map_lookup_elem(&io_uring_ctx_pid, &ioctx);
    if (pid_tgid_ptr) {
        return *pid_tgid_ptr;
    } else {
        return bpf_get_current_pid_tgid();
    }
}

#endif