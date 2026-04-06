#ifndef __SK_CONN_H
#define __SK_CONN_H

#include "ktypes.h"
#include "bpf_helpers.h"

#include "defs.h"
#include "sk.h"
#include "tracer/tracer.h"

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
        conn->conn_stats.duration = bpf_ktime_get_ns() - sk_stats->start_ns;
        conn->conn_stats.direction = sk_stats->direction;

        // offset absolute counters with initial values read
        conn->conn_stats.sent_bytes -= sk_stats->initial.sent_bytes;
        conn->conn_stats.recv_bytes -= sk_stats->initial.recv_bytes;
        conn->conn_stats.sent_packets -= sk_stats->initial.sent_packets;
        conn->conn_stats.recv_packets -= sk_stats->initial.recv_packets;
        conn->tcp_stats.retransmits -= sk_stats->initial.retransmits;
        conn->tcp_stats.reord_seen -= sk_stats->initial.reord_seen;
        conn->tcp_stats.rcv_ooopack -= sk_stats->initial.rcv_ooopack;
        conn->tcp_stats.delivered_ce -= sk_stats->initial.delivered_ce;
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
        conn->conn_stats.duration = bpf_ktime_get_ns() - sk_stats->start_ns;
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
    if (!is_protocol_family_enabled(sk)) {
        return 0;
    }

    conn_t conn = {};
    if (sk->sk_protocol == IPPROTO_TCP || sk->sk_protocol == IPPROTO_MPTCP) {
        log_debug("iterate tcp: sk=%p pid=%d", sk, task->tgid);
        sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, 0);
        if (!create_tcp_conn(&conn, sk, sk_stats, task)) {
            return 0;
        }
    } else if (sk->sk_protocol == IPPROTO_UDP) {
        log_debug("iterate udp: sk=%p pid=%d", sk, task->tgid);
        sk_udp_stats_t *sk_stats = bpf_sk_storage_get(&sk_udp_stats, sk, 0, 0);
        if (!create_udp_conn(&conn, sk, sk_stats, task)) {
            return 0;
        }
    }

    if (!conn.tup.pid) {
        conn.tup.pid = task->tgid;
    }
    bpf_seq_write(seq, &conn, sizeof(conn_t));
    return 0;
}

#endif
