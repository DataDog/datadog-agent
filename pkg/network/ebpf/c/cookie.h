#ifndef __COOKIE_H__
#define __COOKIE_H__

#include "ktypes.h"
#include "bpf_core_read.h"

static __always_inline u32 get_sk_cookie(struct sock *sk) {
    return bpf_get_prandom_u32();
}

#endif // __COOKIE_H__
