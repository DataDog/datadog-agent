#ifndef __SOCK_IMPL_H
#define __SOCK_IMPL_H

#include "bpf_helpers.h"

#include <linux/tcp.h>

static __always_inline void* sock_rtt(struct sock *sk) {
    return &(tcp_sk(sk)->srtt_us);
}

static __always_inline void* sock_rtt_var(struct sock *sk) {
    return &(tcp_sk(sk)->mdev_us);
}

static __always_inline void get_tcp_segment_counts(struct sock* skp, __u32* packets_in, __u32* packets_out) {
    bpf_probe_read_kernel(packets_out, sizeof(*packets_out), &tcp_sk(skp)->segs_out);
    bpf_probe_read_kernel(packets_in, sizeof(*packets_in), &tcp_sk(skp)->segs_in);
}


#endif // __SOCK_IMPL_H
