#ifndef _HELPERS_SELINUX_H_
#define _HELPERS_SELINUX_H_

#include "maps.h"

int __attribute__((always_inline)) fill_selinux_status_payload(struct syscall_cache_t *syscall) {
    // disable
    u32 key = SELINUX_ENFORCE_STATUS_DISABLE_KEY;
    void *ptr = bpf_map_lookup_elem(&selinux_enforce_status, &key);
    if (!ptr) {
        return 0;
    }
    syscall->selinux.payload.status.disable_value = *(u16 *)ptr;

    // enforce
    key = SELINUX_ENFORCE_STATUS_ENFORCE_KEY;
    ptr = bpf_map_lookup_elem(&selinux_enforce_status, &key);
    if (!ptr) {
        return 0;
    }
    syscall->selinux.payload.status.enforce_value = *(u16 *)ptr;

    return 0;
}

#endif
