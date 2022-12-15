#ifndef __COOKIE_H__
#define __COOKIE_H__

#include "ktypes.h"

static __always_inline u32 get_sk_cookie(struct sock *sk) {
#ifdef COMPILE_CORE
    return bpf_get_prandom_u32();
#else
    u64 t = bpf_ktime_get_ns();
    return (u32)((u64)sk ^ t);
#endif
}

#endif // __COOKIE_H__
