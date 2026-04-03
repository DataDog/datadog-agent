#ifndef __SK_INIT_H
#define __SK_INIT_H

#include "ktypes.h"
#include "bpf_helpers.h"

#include "defs.h"
#include "maps.h"
#include "tracer/port.h"
#include "ip.h"
#include "ipv6.h"
#include "netns.h"

static __always_inline void initialize_tcp_socket(struct sock *sk, struct task_struct *task) {
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return;
    }
    struct tcp_sock *tp = bpf_skc_to_tcp_sock(sk);
    if (!tp) {
        return;
    }

    // TODO do any of these need to be stored?
//    conn->tcp_stats.reord_seen = tp->reord_seen;
//    conn->tcp_stats.rcv_ooopack = tp->rcv_ooopack;
//    conn->tcp_stats.delivered_ce = tp->delivered_ce;

    sk_stats->initial_sent_bytes = tp->bytes_sent;
    sk_stats->initial_recv_bytes = tp->bytes_received;
    sk_stats->initial_sent_packets = tp->segs_out;
    sk_stats->initial_recv_packets = tp->segs_in;
    sk_stats->initial_retransmits = tp->total_retrans;

    sk_stats->tup.pid = task->tgid;
    sk_stats->tup.netns = get_netns_from_sock(sk);

    port_binding_t pb = {};
    pb.netns = sk_stats->tup.netns;
    pb.port = read_sport(sk);
    u32 *port_count = bpf_map_lookup_elem(&port_bindings, &pb);
    sk_stats->direction = (port_count != NULL && *port_count > 0) ? CONN_DIRECTION_INCOMING : CONN_DIRECTION_OUTGOING;
}

static __always_inline void initialize_udp_socket(struct sock *sk, struct task_struct *task) {
    sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return;
    }

    sk_stats->tup.pid = task->tgid;
    sk_stats->tup.netns = get_netns_from_sock(sk);

    port_binding_t pb = {};
    pb.netns = sk_stats->tup.netns;
    pb.port = read_sport(sk);
    u32 *port_count = bpf_map_lookup_elem(&udp_port_bindings, &pb);
    sk_stats->direction = (port_count != NULL && *port_count > 0) ? CONN_DIRECTION_INCOMING : CONN_DIRECTION_OUTGOING;
}

SEC("iter/task_file")
int bpf_iter__task_file_initial_sockets(struct bpf_iter__task_file *ctx) {
    struct task_struct *task = ctx->task;
    struct file *file = ctx->file;
    if (!task || !file) {
        return 0;
    }
    struct socket *sock = bpf_sock_from_file(file);
    if (!sock) {
        return 0;
    }
    struct sock *sk = sock->sk;

    if (sk->sk_protocol == IPPROTO_TCP || sk->sk_protocol == IPPROTO_MPTCP) {
        switch (sk->sk_family) {
        case AF_INET6:
            if (!is_tcpv6_enabled()) return 0;
            break;
        case AF_INET:
            if (!is_tcpv4_enabled()) return 0;
            break;
        default:
            return 0;
        }

        initialize_tcp_socket(sk, task);
        return 0;
    } else if (sk->sk_protocol == IPPROTO_UDP) {
        switch (sk->sk_family) {
        case AF_INET6:
            if (!is_udpv6_enabled()) return 0;
            break;
        case AF_INET:
            if (!is_udpv4_enabled()) return 0;
            break;
        default:
            return 0;
        }

        initialize_udp_socket(sk, task);
        return 0;
    }
    return 0;
}

#endif
