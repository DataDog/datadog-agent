#ifndef _HELPERS_IOURING_H_
#define _HELPERS_IOURING_H_

#include "constants/offsets/filesystem.h"
#include "maps.h"

void __attribute__((always_inline)) cache_ioctx_pid_tgid(void *ioctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
#ifdef DEBUG
    bpf_printk("pid = %d", (u32)pid_tgid);
    bpf_printk("tgid = %d", pid_tgid >> 32);
    bpf_printk("ioctx in = %p", ioctx);
#endif
    bpf_map_update_elem(&io_uring_ctx_pid, &ioctx, &pid_tgid, BPF_ANY);
}

u64 __attribute__((always_inline)) get_pid_tgid_from_iouring(void *req) {
    void *ioctx;
    int ret = bpf_probe_read(&ioctx, sizeof(void *), req + get_iokiocb_ctx_offset());
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
