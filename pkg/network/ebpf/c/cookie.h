#ifndef __COOKIE_H__
#define __COOKIE_H__

#include <net/sock.h>

static __always_inline u64 get_socket_cookie(struct sock *sk) {
    return (u64)sk;
}

#endif // __COOKIE_H__
