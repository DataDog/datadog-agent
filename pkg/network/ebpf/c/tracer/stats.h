#ifndef __TRACER_STATS_H
#define __TRACER_STATS_H

#include "bpf_builtins.h"
#include "bpf_core_read.h"
#include "defs.h"

#include "tracer/tracer.h"
#include "tracer/maps.h"
#include "tracer/telemetry.h"
#include "cookie.h"
#include "sock.h"
#include "port_range.h"
#include "protocols/classification/shared-tracer-maps.h"
#include "protocols/classification/stack-helpers.h"
#include "protocols/tls/tags-types.h"
#include "ip.h"
#include "skb.h"

#ifdef COMPILE_PREBUILT
static __always_inline __u64 offset_rtt();
static __always_inline __u64 offset_rtt_var();
#endif

static __always_inline conn_stats_ts_t *get_conn_stats(conn_tuple_t *t, struct sock *sk) {
    conn_stats_ts_t *cs = bpf_map_lookup_elem(&conn_stats, t);
    if (cs) {
        return cs;
    }

    // initialize-if-no-exist the connection stat, and load it
    conn_stats_ts_t empty = {};
    bpf_memset(&empty, 0, sizeof(conn_stats_ts_t));
    empty.duration = bpf_ktime_get_ns();
    empty.cookie = get_sk_cookie(sk);
    bpf_map_update_with_telemetry(conn_stats, t, &empty, BPF_NOEXIST);
    return bpf_map_lookup_elem(&conn_stats, t);
}

static __always_inline void update_conn_state(conn_tuple_t *t, conn_stats_ts_t *stats, size_t sent_bytes, size_t recv_bytes) {
    if (t->metadata & CONN_TYPE_TCP || stats->flags & CONN_ASSURED) {
        return;
    }

    if (stats->recv_bytes == 0 && sent_bytes > 0) {
        stats->flags |= CONN_L_INIT;
        return;
    }

    if (stats->sent_bytes == 0 && recv_bytes > 0) {
        stats->flags |= CONN_R_INIT;
        return;
    }

    // If a three-way "handshake" was established, we mark the connection as assured
    if ((stats->flags & CONN_L_INIT && stats->recv_bytes > 0 && sent_bytes > 0) || (stats->flags & CONN_R_INIT && stats->sent_bytes > 0 && recv_bytes > 0)) {
        stats->flags |= CONN_ASSURED;
    }
}

// this function marks the protocol stack object with the connection direction
//
// *how is the connection direction determined?*
//
// Basically we compare the src-side of the normalized USM tuple (which should
// contain the client port), with the source port of the TCP *socket* (here
// supplied as part the `pre_norm_tuple` argument). If they match, we mark the
// protocol stack with FLAG_CLIENT_SIDE, otherwise we mark it with
// FLAG_SERVER_SIDE.
//
// *why do we do that?*
//
// We do this to mitigate a race condition that may arise in the context of
// localhost traffic when deleting the protocol_stack_t entry. This means that
// we're pretty much only interested in the case where a protocol stack is
// annothed with *both* FLAG_SERVER_SIDE and FLAG_CLIENT_SIDE. For more context
// refer to classification/shared-tracer-maps.h
//
// *what if there is something wrong with the USM normalization?*
//
// This doesn't matter in our case. Even if FLAG_SERVER_SIDE and
// FLAG_CLIENT_SIDE are flipped, all we care about is the case where both flags
// are present.
static __always_inline void mark_protocol_direction(conn_tuple_t *pre_norm_tuple, conn_tuple_t *norm_tuple, protocol_stack_t *protocol_stack) {
    if (pre_norm_tuple->sport == norm_tuple->sport) {
        set_protocol_flag(protocol_stack, FLAG_CLIENT_SIDE);
        return;
    }

    set_protocol_flag(protocol_stack, FLAG_SERVER_SIDE);
}

static __always_inline void update_protocol_classification_information(conn_tuple_t *t, conn_stats_ts_t *stats) {
    if (is_fully_classified(&stats->protocol_stack)) {
        return;
    }

    conn_tuple_t conn_tuple_copy = *t;
    // The classifier is a socket filter and there we are not accessible for pid and netns.
    // The key is based of the source & dest addresses and ports, and the metadata.
    conn_tuple_copy.netns = 0;
    conn_tuple_copy.pid = 0;
    normalize_tuple(&conn_tuple_copy);

    protocol_stack_t *protocol_stack = __get_protocol_stack(&conn_tuple_copy);
    set_protocol_flag(protocol_stack, FLAG_NPM_ENABLED);
    mark_protocol_direction(t, &conn_tuple_copy, protocol_stack);
    merge_protocol_stacks(&stats->protocol_stack, protocol_stack);

    conn_tuple_t *cached_skb_conn_tup_ptr = bpf_map_lookup_elem(&conn_tuple_to_socket_skb_conn_tuple, &conn_tuple_copy);
    if (!cached_skb_conn_tup_ptr) {
        return;
    }

    conn_tuple_copy = *cached_skb_conn_tup_ptr;
    protocol_stack = __get_protocol_stack(&conn_tuple_copy);
    set_protocol_flag(protocol_stack, FLAG_NPM_ENABLED);
    mark_protocol_direction(t, &conn_tuple_copy, protocol_stack);
    merge_protocol_stacks(&stats->protocol_stack, protocol_stack);
}

static __always_inline void determine_connection_direction(conn_tuple_t *t, conn_stats_ts_t *conn_stats) {
    if (conn_stats->direction != CONN_DIRECTION_UNKNOWN) {
        return;
    }

    u32 *port_count = NULL;
    port_binding_t pb = {};
    pb.port = t->sport;
    pb.netns = t->netns;
    if (t->metadata & CONN_TYPE_TCP) {
        port_count = bpf_map_lookup_elem(&port_bindings, &pb);
    } else {
        port_count = bpf_map_lookup_elem(&udp_port_bindings, &pb);
    }
    conn_stats->direction = (port_count != NULL && *port_count > 0) ? CONN_DIRECTION_INCOMING : CONN_DIRECTION_OUTGOING;
}

// update_conn_stats update the connection metadata : protocol, tags, timestamp, direction, packets, bytes sent and received
static __always_inline void update_conn_stats(conn_tuple_t *t, size_t sent_bytes, size_t recv_bytes, u64 ts, conn_direction_t dir,
    __u32 packets_out, __u32 packets_in, packet_count_increment_t segs_type, struct sock *sk) {
    conn_stats_ts_t *val = NULL;
    val = get_conn_stats(t, sk);
    if (!val) {
        return;
    }

    update_protocol_classification_information(t, val);

    // If already in our map, increment size in-place
    update_conn_state(t, val, sent_bytes, recv_bytes);
    if (sent_bytes) {
        __sync_fetch_and_add(&val->sent_bytes, sent_bytes);
    }
    if (recv_bytes) {
        __sync_fetch_and_add(&val->recv_bytes, recv_bytes);
    }
    if (packets_in) {
        if (segs_type == PACKET_COUNT_INCREMENT) {
            __sync_fetch_and_add(&val->recv_packets, packets_in);
        } else if (segs_type == PACKET_COUNT_ABSOLUTE) {
            val->recv_packets = packets_in;
        }
    }
    if (packets_out) {
        if (segs_type == PACKET_COUNT_INCREMENT) {
            __sync_fetch_and_add(&val->sent_packets, packets_out);
        } else if (segs_type == PACKET_COUNT_ABSOLUTE) {
            val->sent_packets = packets_out;
        }
    }
    val->timestamp = ts;

    if (dir != CONN_DIRECTION_UNKNOWN) {
        val->direction = dir;
    } else {
        determine_connection_direction(t, val);
    }
}

// update_tcp_stats update rtt, retransmission and state on of a TCP connection
static __always_inline void update_tcp_stats(conn_tuple_t *t, tcp_stats_t stats) {
    // initialize-if-no-exist the connection state, and load it
    tcp_stats_t empty = {};
    bpf_map_update_with_telemetry(tcp_stats, t, &empty, BPF_NOEXIST);

    tcp_stats_t *val = bpf_map_lookup_elem(&tcp_stats, t);
    if (val == NULL) {
        return;
    }

    if (stats.rtt > 0) {
        // For more information on the bit shift operations see:
        // https://elixir.bootlin.com/linux/v4.6/source/net/ipv4/tcp.c#L2686
        val->rtt = stats.rtt >> 3;
        val->rtt_var = stats.rtt_var >> 2;
    }

    if (stats.state_transitions > 0) {
        val->state_transitions |= stats.state_transitions;
    }
}

static __always_inline int handle_message(conn_tuple_t *t, size_t sent_bytes, size_t recv_bytes, conn_direction_t dir,
    __u32 packets_out, __u32 packets_in, packet_count_increment_t segs_type, struct sock *sk) {
    u64 ts = bpf_ktime_get_ns();
    update_conn_stats(t, sent_bytes, recv_bytes, ts, dir, packets_out, packets_in, segs_type, sk);
    return 0;
}

static __always_inline int handle_retransmit(struct sock *sk, int count) {
    conn_tuple_t t = {};
    u64 zero = 0;
    if (!read_conn_tuple(&t, sk, zero, CONN_TYPE_TCP)) {
        return 0;
    }

    // initialize-if-no-exist the connection state, and load it
    u32 u32_zero = 0;
    bpf_map_update_with_telemetry(tcp_retransmits, &t, &u32_zero, BPF_NOEXIST);
    u32 *val = bpf_map_lookup_elem(&tcp_retransmits, &t);
    if (val == NULL) {
        return 0;
    }

    __sync_fetch_and_add(val, count);

    return 0;
}

static __always_inline void handle_tcp_stats(conn_tuple_t* t, struct sock* sk, u8 state) {
    u32 rtt = 0, rtt_var = 0;
#ifdef COMPILE_PREBUILT
    bpf_probe_read_kernel(&rtt, sizeof(rtt), (char*)sk + offset_rtt());
    bpf_probe_read_kernel(&rtt_var, sizeof(rtt_var), (char*)sk + offset_rtt_var());
#else
    BPF_CORE_READ_INTO(&rtt, tcp_sk(sk), srtt_us);
    BPF_CORE_READ_INTO(&rtt_var, tcp_sk(sk), mdev_us);
#endif

    tcp_stats_t stats = { .rtt = rtt, .rtt_var = rtt_var };
    if (state > 0) {
        stats.state_transitions = (1 << state);
    }
    update_tcp_stats(t, stats);
}

static __always_inline int handle_skb_consume_udp(struct sock *sk, struct sk_buff *skb, int len) {
    if (len < 0) {
        // peeking or an error happened
        return 0;
    }

    u64 pid_tgid = bpf_get_current_pid_tgid();
    udp_recv_sock_t *st = bpf_map_lookup_elem(&udp_recv_sock, &pid_tgid);
    if (!st) { // no entry means a peek
        return 0;
    }

    conn_tuple_t t;
    bpf_memset(&t, 0, sizeof(conn_tuple_t));
    int data_len = sk_buff_to_tuple(skb, &t);
    if (data_len <= 0) {
        log_debug("ERR(skb_consume_udp): error reading tuple ret=%d", data_len);
        return 0;
    }
    // we are receiving, so we want the daddr to become the laddr
    flip_tuple(&t);

    log_debug("skb_consume_udp: bytes=%d", data_len);
    t.pid = pid_tgid >> 32;
    t.netns = get_netns_from_sock(sk);
    return handle_message(&t, 0, data_len, CONN_DIRECTION_UNKNOWN, 0, 1, PACKET_COUNT_INCREMENT, sk);
}

static __always_inline int handle_tcp_recv(u64 pid_tgid, struct sock *skp, int recv) {
    conn_tuple_t t = {};
    if (!read_conn_tuple(&t, skp, pid_tgid, CONN_TYPE_TCP)) {
        return 0;
    }

    handle_tcp_stats(&t, skp, 0);

    __u32 packets_in = 0;
    __u32 packets_out = 0;
    get_tcp_segment_counts(skp, &packets_in, &packets_out);

    return handle_message(&t, 0, recv, CONN_DIRECTION_UNKNOWN, packets_out, packets_in, PACKET_COUNT_ABSOLUTE, skp);
}

#endif // __TRACER_STATS_H
