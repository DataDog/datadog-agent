#ifndef __COOKIE_H__
#define __COOKIE_H__

#include "ktypes.h"
#include "bpf_helpers.h"

#include "defs.h"

static __always_inline u32 get_sk_cookie(struct sock *sk) {
#if defined(COMPILE_RUNTIME) || defined(COMPILE_CORE)
    if (bpf_helper_exists(BPF_FUNC_get_prandom_u32)) {
        return bpf_get_prandom_u32();
    }
#endif

    __u64 t = bpf_ktime_get_ns();
    __u64 _sk = 0;
    bpf_probe_read_kernel_with_telemetry(&_sk, sizeof(_sk), &sk);
    return (u32)(_sk ^ t);
}

#endif // __COOKIE_H__
