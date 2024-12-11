#ifndef __NATIVE_TLS_MAPS_H
#define __NATIVE_TLS_MAPS_H

#include "map-defs.h"

BPF_LRU_MAP(ssl_sock_by_ctx, void *, ssl_sock_t, 1)

BPF_LRU_MAP(ssl_read_args, u64, ssl_read_args_t, 1024)

BPF_LRU_MAP(ssl_read_ex_args, u64, ssl_read_ex_args_t, 1024)

BPF_LRU_MAP(ssl_write_args, u64, ssl_write_args_t, 1024)

BPF_LRU_MAP(ssl_write_ex_args, u64, ssl_write_ex_args_t, 1024)

BPF_LRU_MAP(bio_new_socket_args, __u64, __u32, 1024)

BPF_LRU_MAP(fd_by_ssl_bio, __u32, void *, 1024)

BPF_LRU_MAP(ssl_ctx_by_pid_tgid, __u64, void *, 1024)

#endif
