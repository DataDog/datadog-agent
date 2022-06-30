#ifndef __SOCK_DEFINES_H__
#define __SOCK_DEFINES_H__

#include "bpf_helpers.h"

#include <net/sock.h>

static __always_inline void* sock_rtt(struct sock *sk);
static __always_inline void* sock_rtt_var(struct sock *sk);

static __always_inline void get_tcp_segment_counts(struct sock* skp, __u32* packets_in, __u32* packets_out);
static __always_inline int read_conn_tuple(conn_tuple_t *t, struct sock *skp, u64 pid_tgid, metadata_mask_t type);


#endif // __SOCK_DEFINES_H__
