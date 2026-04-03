#ifndef __SK_TCP_H
#define __SK_TCP_H

#include "bpf_helpers.h"
#include "bpf_endian.h"

#include "defs.h"
#include "maps.h"
#include "sock.h"
#include "tracer/tracer.h"
#include "tracer/port.h"
#include "sk.h"

__maybe_unused static __always_inline bool tcp_failed_connections_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("tcp_failed_connections_enabled", val);
    return val > 0;
}

static __always_inline int create_tcp_conn(conn_t *conn, struct sock *sk, sk_tcp_stats_t *sk_stats, struct task_struct *task) {
    struct tcp_sock *tp = bpf_skc_to_tcp_sock(sk);
    if (!tp) {
        return 0;
    }

    if (sk_stats) {
        copy_conn_tuple(&conn->tup, &sk_stats->tup);
    }
    if (!read_conn_tuple_sk(&conn->tup, sk, task)) {
        return 0;
    }
    conn->tup.metadata |= CONN_TYPE_TCP;

    conn->tcp_stats.rtt = tp->srtt_us >> 3;
    conn->tcp_stats.rtt_var = tp->mdev_us >> 2;
    conn->tcp_stats.retransmits = tp->total_retrans;
    conn->tcp_stats.reord_seen = tp->reord_seen;
    conn->tcp_stats.rcv_ooopack = tp->rcv_ooopack;
    conn->tcp_stats.delivered_ce = tp->delivered_ce;
    conn->tcp_stats.ecn_negotiated = tp->ecn_flags & TCP_ECN_OK;

    conn->conn_stats.sent_bytes = tp->bytes_sent;
    conn->conn_stats.recv_bytes = tp->bytes_received;
    conn->conn_stats.sent_packets = tp->segs_out;
    conn->conn_stats.recv_packets = tp->segs_in;
    conn->conn_stats.timestamp = tp->tcp_mstamp * NSEC_PER_USEC;
    conn->conn_stats.cookie = (__u32)(bpf_get_socket_cookie(sk) & 0xFFFFFFFF);
    // TODO conn->conn_stats.protocol_stack
    // TODO conn->conn_stats.tls_tags
    // TODO conn->conn_stats.cert_id

    if (sk_stats) {
        conn->tcp_stats.failure_reason = sk_stats->failure_reason;
        conn->tcp_stats.state_transitions = sk_stats->state_transitions;
        conn->tcp_stats.tcp_event_stats = sk_stats->tcp_event_stats;
        conn->conn_stats.duration_ms = sk_stats->start_ms;
        conn->conn_stats.direction = sk_stats->direction;

        // offset absolute counters with initial values read
        conn->conn_stats.sent_bytes -= sk_stats->initial_sent_bytes;
        conn->conn_stats.recv_bytes -= sk_stats->initial_recv_bytes;
        conn->conn_stats.sent_packets -= sk_stats->initial_sent_packets;
        conn->conn_stats.recv_packets -= sk_stats->initial_recv_packets;
        conn->tcp_stats.retransmits -= sk_stats->initial_retransmits;
    }
    return 1;
}

static __always_inline int create_udp_conn(conn_t *conn, struct sock *sk, sk_udp_stats_t *sk_stats, struct task_struct *task) {
    if (sk_stats) {
        copy_conn_tuple(&conn->tup, &sk_stats->tup);
    }
    if (!read_conn_tuple_sk(&conn->tup, sk, task)) {
        return 0;
    }
    conn->tup.metadata |= CONN_TYPE_UDP;

//    struct net *nt = sk->sk_net.net;
//    conn->conn_stats.sent_packets = nt->mib.udp_statistics->mibs[UDP_MIB_OUTDATAGRAMS];
//    conn->conn_stats.recv_packets = nt->mib.udp_statistics->mibs[UDP_MIB_INDATAGRAMS];

    conn->conn_stats.cookie = (__u32)(bpf_get_socket_cookie(sk) & 0xFFFFFFFF);
    // TODO conn->conn_stats.protocol_stack
    // TODO conn->conn_stats.tls_tags
    // TODO conn->conn_stats.cert_id

    if (sk_stats) {
        conn->conn_stats.duration = sk_stats->start_ns;
        conn->conn_stats.direction = sk_stats->direction;
        conn->conn_stats.sent_bytes = sk_stats->sent_bytes;
        conn->conn_stats.recv_bytes = sk_stats->recv_bytes;
        conn->conn_stats.sent_packets = sk_stats->sent_packets;
        conn->conn_stats.recv_packets = sk_stats->recv_packets;
        conn->conn_stats.timestamp = sk_stats->timestamp_ns;
        conn->conn_stats.flags = sk_stats->flags;
    }
    return 1;
}

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
int bpf_iter__task_file_port_bindings(struct bpf_iter__task_file *ctx) {
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

        if (sk->__sk_common.skc_state == TCP_LISTEN) {
            port_binding_t pb = {};
            pb.netns = get_netns_from_sock(sk);
            pb.port = read_sport(sk);
            add_port_bind(&pb, port_bindings);
        }
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

        // TODO skip if in ephemeral port range
        if (sk->__sk_common.skc_state == TCP_CLOSE) {
            port_binding_t pb = {};
            pb.netns = get_netns_from_sock(sk);
            pb.port = read_sport(sk);
            add_port_bind(&pb, udp_port_bindings);
        }
        return 0;
    }
    return 0;
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

SEC("iter/task_file")
int bpf_iter__task_file_socket(struct bpf_iter__task_file *ctx) {
    struct task_struct *task = ctx->task;
    struct file *file = ctx->file;
    struct seq_file *seq = ctx->meta->seq;
    if (!task || !file) {
        return 0;
    }
    struct socket *sock = bpf_sock_from_file(file);
    if (!sock) {
        return 0;
    }
    struct sock *sk = sock->sk;

    conn_t conn = {};
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

        log_debug("iterate tcp: sk=%pK pid=%d", sk, task->tgid);
        sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, 0);
        if (!create_tcp_conn(&conn, sk, sk_stats, task)) {
            return 0;
        }
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

        log_debug("iterate udp: sk=%pK pid=%d", sk, task->tgid);
        sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, 0);
        if (!create_udp_conn(&conn, sk, sk_stats, task)) {
            return 0;
        }
    } else {
        return 0;
    }

    if (!conn.tup.pid) {
        conn.tup.pid = task->tgid;
    }
    bpf_seq_write(seq, &conn, sizeof(conn_t));
    return 0;
}

// get_tcp_failure returns an error code for tcp_done/tcp_close, if there was one
static __always_inline int get_tcp_failure(struct sock *sk) {
    int err = sk->sk_err;
    if (err != 0) {
        return err;
    }

    // we are still in SYN_SENT when the socket closed, meaning the connect was cancelled
    if (sk->__sk_common.skc_state == TCP_SYN_SENT) {
        return TCP_CONN_FAILED_CANCELED;
    }
    return 0;
}

static __always_inline bool is_tcp_failure_recognized(int err) {
    switch(err) {
        case TCP_CONN_FAILED_RESET:
        case TCP_CONN_FAILED_TIMEOUT:
        case TCP_CONN_FAILED_REFUSED:
        case TCP_CONN_FAILED_EHOSTUNREACH:
        case TCP_CONN_FAILED_ENETUNREACH:
        case TCP_CONN_FAILED_CANCELED:
            return true;
        default:
            return false;
    }
}

static __always_inline bool handle_sk_tcp_failure(struct sock *sk, sk_tcp_stats_t *sk_stats) {
    if (!tcp_failed_connections_enabled()) {
        return false;
    }

    int err = get_tcp_failure(sk);
    if (!err) {
        return false;
    }
    log_debug("tcp failure: sk=%p err=%d", sk, err);
    if (!is_tcp_failure_recognized(err)) {
        return false;
    }
    sk_stats->failure_reason = err;
    return true;
}

static __always_inline void sockops_tcp_close(struct bpf_sock_ops *ctx, struct sock *sk, sk_tcp_stats_t *sk_stats) {
    log_debug("sockops: sk=%p state=TCP_CLOSE", sk);
    sk_stats->state_transitions |= (1 << TCP_CLOSE);
}

SEC("sockops")
int tcp_sockops(struct bpf_sock_ops *ctx) {
    if (!ctx->is_fullsock) {
        return 1;
    }
    struct bpf_sock *bpf_sk = ctx->sk;
    if (!bpf_sk) {
        return 1;
    }
    struct sock *sk = (struct sock *)bpf_skc_to_tcp_sock(bpf_sk);
    if (!sk) {
        return 1;
    }

    log_debug("sockops: sk=%p op=%u", sk, ctx->op);
    switch (ctx->op) {
    case BPF_SOCK_OPS_TCP_CONNECT_CB:
        bpf_sock_ops_cb_flags_set(ctx, BPF_SOCK_OPS_STATE_CB_FLAG);
        return 1;
    case BPF_SOCK_OPS_STATE_CB:
    case BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB:
    case BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB:
        break;
    default:
        return 1;
    }

    log_debug("sockops: sk=%p lport=%u rport=%u", sk, ctx->local_port, bpf_ntohl(ctx->remote_port));
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 1;
    }

    if (ctx->local_port) sk_stats->tup.sport = ctx->local_port;
    if (ctx->remote_port) sk_stats->tup.dport = bpf_ntohl(ctx->remote_port);
    if (ctx->family) {
        if (ctx->family == AF_INET) {
            sk_stats->tup.metadata |= CONN_V4;
            if (ctx->local_ip4) {
                sk_stats->tup.saddr_h = 0;
                sk_stats->tup.saddr_l = ctx->local_ip4;
            }
            if (ctx->remote_ip4) {
                sk_stats->tup.daddr_h = 0;
                sk_stats->tup.daddr_l = ctx->remote_ip4;
            }
        } else if (ctx->family == AF_INET6) {
            sk_stats->tup.metadata |= CONN_V6;
            struct in6_addr saddr;
            saddr.in6_u.u6_addr32[0] = ctx->local_ip6[0];
            saddr.in6_u.u6_addr32[1] = ctx->local_ip6[1];
            saddr.in6_u.u6_addr32[2] = ctx->local_ip6[2];
            saddr.in6_u.u6_addr32[3] = ctx->local_ip6[3];
            read_in6_addr(&sk_stats->tup.saddr_h, &sk_stats->tup.saddr_l, &saddr);

            struct in6_addr daddr;
            daddr.in6_u.u6_addr32[0] = ctx->remote_ip6[0];
            daddr.in6_u.u6_addr32[1] = ctx->remote_ip6[1];
            daddr.in6_u.u6_addr32[2] = ctx->remote_ip6[2];
            daddr.in6_u.u6_addr32[3] = ctx->remote_ip6[3];
            read_in6_addr(&sk_stats->tup.daddr_h, &sk_stats->tup.daddr_l, &daddr);
        }
    }
    if (!sk_stats->tup.netns) {
        sk_stats->tup.netns = get_netns_from_sock(sk);
    }

    if (ctx->op == BPF_SOCK_OPS_STATE_CB) {
        switch (ctx->state) {
        case BPF_TCP_ESTABLISHED:
            log_debug("sockops: sk=%p state=TCP_ESTABLISHED", sk);
            sk_stats->state_transitions |= (1 << TCP_ESTABLISHED);
            break;
        case BPF_TCP_CLOSE:
            sockops_tcp_close(ctx, sk, sk_stats);
            break;
        }
    } else if (ctx->op == BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB) {
        sk_stats->direction = CONN_DIRECTION_OUTGOING;
    } else if (ctx->op == BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB) {
        sk_stats->direction = CONN_DIRECTION_INCOMING;
    }

    return 1;
}

SEC("fentry/tcp_connect")
int BPF_PROG(tcp_connect_entry, struct sock *sk) {
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_connect: sk=%p", sk);
    sk_stats->tup.pid = GET_USER_MODE_PID(bpf_get_current_pid_tgid());
    sk_stats->start_ns = bpf_ktime_get_ns();
    sk_stats->direction = CONN_DIRECTION_OUTGOING;
    return 0;
}

SEC("fexit/inet_csk_accept")
int BPF_PROG(inet_csk_accept_exit, struct sock *orig_sk, int flags, int *err, bool kern, struct sock *sk) {
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }
    log_debug("inet_csk_accept: sk=%p", sk);
    sk_stats->tup.pid = GET_USER_MODE_PID(bpf_get_current_pid_tgid());
    sk_stats->start_ns = bpf_ktime_get_ns();
    sk_stats->direction = CONN_DIRECTION_INCOMING;
    sk_stats->state_transitions |= (1 << TCP_ESTABLISHED);
    return 0;
}

SEC("fentry/tcp_finish_connect")
int BPF_PROG(tcp_finish_connect_entry, struct sock *sk) {
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_finish_connect: sk=%p", sk);
    sk_stats->state_transitions |= (1 << TCP_ESTABLISHED);
    return 0;
}

SEC("fentry/tcp_done")
int BPF_PROG(tcp_done_entry, struct sock *sk) {
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_done: sk=%p", sk);
    handle_sk_tcp_failure(sk, sk_stats);
    sk_stats->state_transitions |= (1 << TCP_CLOSE);

    conn_t conn = {};
    if (!create_tcp_conn(&conn, sk, sk_stats, NULL)) {
        return 0;
    }

    conn.conn_stats.duration = bpf_ktime_get_ns() - conn.conn_stats.duration;
    bpf_ringbuf_output(&conn_close_event, &conn, sizeof(conn_t), get_ringbuf_flags(sizeof(conn_t)));
    return 0;
}

SEC("fentry/tcp_close")
int BPF_PROG(tcp_close_entry, struct sock *sk) {
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_close: sk=%p", sk);
    handle_sk_tcp_failure(sk, sk_stats);
    sk_stats->state_transitions |= (1 << TCP_CLOSE);

    conn_t conn = {};
    if (!create_tcp_conn(&conn, sk, sk_stats, NULL)) {
        return 0;
    }

    conn.conn_stats.duration = bpf_ktime_get_ns() - conn.conn_stats.duration;
    bpf_ringbuf_output(&conn_close_event, &conn, sizeof(conn_t), get_ringbuf_flags(sizeof(conn_t)));
    return 0;
}

SEC("fentry/tcp_enter_loss")
int BPF_PROG(tcp_enter_loss_entry, struct sock *sk) {
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_enter_loss: sk=%p", sk);
    sk_stats->tcp_event_stats.rto_count += 1;
    return 0;
}

SEC("fentry/tcp_enter_recovery")
int BPF_PROG(tcp_enter_recovery_entry, struct sock *sk) {
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_enter_recovery: sk=%p", sk);
    sk_stats->tcp_event_stats.recovery_count += 1;
    return 0;
}

SEC("fentry/tcp_send_probe0")
int BPF_PROG(tcp_send_probe0_entry, struct sock *sk) {
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, BPF_SK_STORAGE_GET_F_CREATE);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_send_probe0: sk=%p", sk);
    sk_stats->tcp_event_stats.probe0_count += 1;
    return 0;
}

#endif
