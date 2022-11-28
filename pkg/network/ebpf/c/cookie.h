#ifndef __COOKIE_H__
#define __COOKIE_H__

#include <linux/types.h>
#include <net/sock.h>

static __always_inline u32 get_sk_cookie(struct sock *sk) {
    u64 t = bpf_ktime_get_ns();
    return (u32)((u64)sk ^ t);
}

#endif // __COOKIE_H__
