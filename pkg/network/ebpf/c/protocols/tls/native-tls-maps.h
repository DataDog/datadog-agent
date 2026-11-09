#ifndef __NATIVE_TLS_MAPS_H
#define __NATIVE_TLS_MAPS_H

#include "map-defs.h"
#include "pid_tgid.h"

BPF_HASH_MAP(ssl_sock_by_ctx, ssl_ctx_pid_tgid_t, ssl_sock_t, 1)

// Builds the ssl_sock_by_ctx key. The key is per process, not per thread: any thread of
// the process may use or close an SSL context, and the delete paths only see the thread
// that closes it.
static __always_inline ssl_ctx_pid_tgid_t ssl_sock_by_ctx_key(__u64 pid_tgid, void *ssl_ctx) {
    ssl_ctx_pid_tgid_t key = {
        .pid = GET_USER_MODE_PID(pid_tgid),
        .pad = 0,
        .ctx = ssl_ctx,
    };
    return key;
}

BPF_HASH_MAP(ssl_ctx_by_tuple, conn_tuple_t, void *, 1)

BPF_HASH_MAP(ssl_read_args, u64, ssl_read_args_t, 1024)

BPF_HASH_MAP(ssl_read_ex_args, u64, ssl_read_ex_args_t, 1024)

BPF_HASH_MAP(ssl_write_args, u64, ssl_write_args_t, 1024)

BPF_HASH_MAP(ssl_write_ex_args, u64, ssl_write_ex_args_t, 1024)

BPF_HASH_MAP(bio_new_socket_args, __u64, __u32, 1024)

BPF_HASH_MAP(fd_by_ssl_bio, void*, void *, 1024)

BPF_HASH_MAP(ssl_ctx_by_pid_tgid, __u64, void *, 1)

#endif
