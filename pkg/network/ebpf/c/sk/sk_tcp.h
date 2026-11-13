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
#include "sk_conn.h"

__maybe_unused static __always_inline bool tcp_failed_connections_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("tcp_failed_connections_enabled", val);
    return val > 0;
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

// send_tcp_close_event sends the close event for the current connection on a socket, at most once.
//
// The event is sent when the kernel is done with the connection, not when the application
// closes it, so that it carries the final stats: after tcp_close, an orphaned socket keeps
// sending the data still queued (and retransmitting it), and can still fail (a RST or a
// timeout in FIN_WAIT/LAST_ACK). That point is either:
// - tcp_v4_destroy_sock (used for IPv6 too), which every TCP socket goes through exactly once.
//   Until then, an orphaned socket is listed as active by bpf_iter__sk_storage_map_tcp_orphans,
//   since it no longer has a file for bpf_iter__task_file_socket to find.
// - tcp_disconnect, which zeroes the kernel's per-connection counters, so they have to be read
//   before. It ends the connection when the application closes with SO_LINGER 0, or a connect
//   fails or is cancelled, or the socket is disconnected with connect(AF_UNSPEC). The socket
//   itself can then be connected again, see reset_sk_tcp_stats.
// Sending an event for both tcp_close and tcp_done (as both happen for nearly every connection,
// in either order, and possibly far apart) made userspace report a connection closed twice,
// double counting all of its stats, when the two events straddled a check.
static __always_inline void send_tcp_close_event(struct sock *sk, sk_tcp_stats_t *sk_stats) {
    if (sk_stats->close_sent) {
        log_debug("tcp close event already sent: sk=%p", sk);
        return;
    }
    conn_t conn = {};
    if (create_tcp_conn(&conn, sk, sk_stats, NULL)) {
        bpf_ringbuf_output(&conn_close_event, &conn, sizeof(conn_t), get_ringbuf_flags(sizeof(conn_t)));
    }
    // Only set once the event is in the ring buffer: from here on the iterators skip the socket,
    // and userspace flushes the ring buffer after iterating, so a check sees the connection either
    // as active or as closed. Set even if the event couldn't be sent, so the socket isn't listed
    // as active forever.
    sk_stats->close_sent = 1;
}

// reset_sk_tcp_stats starts tracking a new connection on a socket that was already used for
// one, which tcp_disconnect makes possible.
static __always_inline void reset_sk_tcp_stats(struct sock *sk, sk_tcp_stats_t *sk_stats) {
    struct tcp_sock *tp = bpf_skc_to_tcp_sock(sk);
    if (!tp) {
        return;
    }

    // tcp_disconnect already zeroed these, but they are offsets, so stay consistent anyway
    sk_stats->initial.sent_bytes = tp->bytes_sent;
    sk_stats->initial.recv_bytes = tp->bytes_received;
    sk_stats->initial.sent_packets = tp->segs_out;
    sk_stats->initial.recv_packets = tp->segs_in;
    sk_stats->initial.retransmits = tp->total_retrans;
    sk_stats->initial.reord_seen = tp->reord_seen;
    sk_stats->initial.rcv_ooopack = tp->rcv_ooopack;
    sk_stats->initial.delivered_ce = tp->delivered_ce;

    __u32 netns = sk_stats->tup.netns;
    sk_stats->tup = (conn_tuple_t){};
    sk_stats->tup.netns = netns;
    sk_stats->state_transitions = 0;
    sk_stats->failure_reason = 0;
    sk_stats->tcp_event_stats = (tcp_event_stats_t){};
    sk_stats->close_sent = 0;
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
    switch (ctx->family) {
    case AF_INET:
        if (!is_tcpv4_enabled()) return 1;
        break;
    case AF_INET6:
        if (!is_tcpv6_enabled()) return 1;
        break;
    default:
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
        break;
    case BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB:
    case BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB:
        bpf_sock_ops_cb_flags_set(ctx, BPF_SOCK_OPS_STATE_CB_FLAG);
        break;
    default:
        return 1;
    }

    log_debug("sockops: sk=%p lport=%u rport=%u", sk, ctx->local_port, bpf_ntohl(ctx->remote_port));
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get_or_create(sk_tcp_stats, sk, 0);
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
    if (!is_tcp_family_enabled(sk)) {
        return 0;
    }
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get_or_create(sk_tcp_stats, sk, 0);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_connect: sk=%p", sk);
    if (sk_stats->close_sent || sk_stats->state_transitions) {
        // the socket was used for an earlier connection
        reset_sk_tcp_stats(sk, sk_stats);
    }
    sk_stats->tup.pid = GET_USER_MODE_PID(bpf_get_current_pid_tgid());
    sk_stats->start_ns = bpf_ktime_get_ns();
    sk_stats->direction = CONN_DIRECTION_OUTGOING;
    return 0;
}

SEC("fexit/inet_csk_accept")
int BPF_PROG(inet_csk_accept_exit_pre_6_10_0, struct sock *orig_sk, int flags, int *err, bool kern, struct sock *sk) {
    if (!is_tcp_family_enabled(sk)) {
        return 0;
    }
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get_or_create(sk_tcp_stats, sk, 0);
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

SEC("fexit/inet_csk_accept")
int BPF_PROG(inet_csk_accept_exit, struct sock *orig_sk, struct proto_accept_arg *arg, struct sock *sk) {
    if (!is_tcp_family_enabled(sk)) {
        return 0;
    }
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get_or_create(sk_tcp_stats, sk, 0);
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
    if (!is_tcp_family_enabled(sk)) {
        return 0;
    }
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get_or_create(sk_tcp_stats, sk, 0);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_finish_connect: sk=%p", sk);
    sk_stats->state_transitions |= (1 << TCP_ESTABLISHED);
    return 0;
}

SEC("fentry/tcp_done")
int BPF_PROG(tcp_done_entry, struct sock *sk) {
    if (!is_tcp_family_enabled(sk)) {
        return 0;
    }
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get_or_create(sk_tcp_stats, sk, 0);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_done: sk=%p", sk);
    // the close event is sent later, when the socket is destroyed, see send_tcp_close_event
    handle_sk_tcp_failure(sk, sk_stats);
    sk_stats->state_transitions |= (1 << TCP_CLOSE);
    return 0;
}

SEC("fentry/tcp_close")
int BPF_PROG(tcp_close_entry, struct sock *sk) {
    if (!is_tcp_family_enabled(sk)) {
        return 0;
    }
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get_or_create(sk_tcp_stats, sk, 0);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_close: sk=%p", sk);
    // The close event is sent later, when the socket is destroyed, see send_tcp_close_event.
    // The connection only reaches TCP_CLOSE then, so until that point it's listed as active,
    // and not closed, by bpf_iter__sk_storage_map_tcp_orphans.
    handle_sk_tcp_failure(sk, sk_stats);
    return 0;
}

SEC("fentry/tcp_disconnect")
int BPF_PROG(tcp_disconnect_entry, struct sock *sk) {
    if (!is_tcp_family_enabled(sk)) {
        return 0;
    }
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, 0);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_disconnect: sk=%p", sk);
    handle_sk_tcp_failure(sk, sk_stats);
    sk_stats->state_transitions |= (1 << TCP_CLOSE);
    send_tcp_close_event(sk, sk_stats);
    return 0;
}

SEC("fentry/tcp_v4_destroy_sock")
int BPF_PROG(tcp_v4_destroy_sock_entry, struct sock *sk) {
    if (!is_tcp_family_enabled(sk)) {
        return 0;
    }
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get(&sk_tcp_stats, sk, 0, 0);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_v4_destroy_sock: sk=%p", sk);
    handle_sk_tcp_failure(sk, sk_stats);
    sk_stats->state_transitions |= (1 << TCP_CLOSE);
    send_tcp_close_event(sk, sk_stats);
    return 0;
}

SEC("fentry/tcp_enter_loss")
int BPF_PROG(tcp_enter_loss_entry, struct sock *sk) {
    if (!is_tcp_family_enabled(sk)) {
        return 0;
    }
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get_or_create(sk_tcp_stats, sk, 0);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_enter_loss: sk=%p", sk);
    sk_stats->tcp_event_stats.rto_count += 1;
    return 0;
}

SEC("fentry/tcp_enter_recovery")
int BPF_PROG(tcp_enter_recovery_entry, struct sock *sk) {
    if (!is_tcp_family_enabled(sk)) {
        return 0;
    }
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get_or_create(sk_tcp_stats, sk, 0);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_enter_recovery: sk=%p", sk);
    sk_stats->tcp_event_stats.recovery_count += 1;
    return 0;
}

SEC("fentry/tcp_send_probe0")
int BPF_PROG(tcp_send_probe0_entry, struct sock *sk) {
    if (!is_tcp_family_enabled(sk)) {
        return 0;
    }
    sk_tcp_stats_t *sk_stats = bpf_sk_storage_get_or_create(sk_tcp_stats, sk, 0);
    if (!sk_stats) {
        return 0;
    }
    log_debug("tcp_send_probe0: sk=%p", sk);
    sk_stats->tcp_event_stats.probe0_count += 1;
    return 0;
}

#endif
