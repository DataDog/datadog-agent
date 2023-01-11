#ifndef __COOKIE_H__
#define __COOKIE_H__

#include "ktypes.h"
#include "bpf_core_read.h"

static __always_inline u32 get_sk_cookie(struct sock *sk) {
#if defined(RUNTIME_COMPILED) && LINUX_VERSION_CODE >= KERNEL_VERSION(4, 9, 0)
    return bpf_get_prandom_u32();
#endif

    u64 t = bpf_ktime_get_ns();
    return (u32)((u64)sk ^ t);
}

#endif // __COOKIE_H__
