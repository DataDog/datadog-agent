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
#include "pid_tgid.h"
#include "timestamp_ms.h"
#include "protocols/tls/tls-certs.h"

#ifdef COMPILE_PREBUILT
static __always_inline __u64 offset_rtt();
static __always_inline __u64 offset_rtt_var();
#endif

static __always_inline tls_info_t* get_tls_enhanced_tags(conn_tuple_t* tuple) {
    conn_tuple_t normalized_tup = *tuple;
    normalize_tuple(&normalized_tup);
    tls_info_wrapper_t *wrapper = bpf_map_lookup_elem(&tls_enhanced_tags, &normalized_tup);
    if (!wrapper) {
        return NULL;
    }
    wrapper->updated = bpf_ktime_get_ns();
    return &wrapper->info;
}

static __always_inline tls_info_t* get_or_create_tls_enhanced_tags(conn_tuple_t *tuple) {
    tls_info_t *tags = get_tls_enhanced_tags(tuple);
    if (!tags) {
        conn_tuple_t normalized_tup = *tuple;
        normalize_tuple(&normalized_tup);
        tls_info_wrapper_t empty_tags_wrapper = {};
        empty_tags_wrapper.updated = bpf_ktime_get_ns();

        bpf_map_update_with_telemetry(tls_enhanced_tags, &normalized_tup, &empty_tags_wrapper, BPF_ANY);
        tls_info_wrapper_t *wrapper_ptr = bpf_map_lookup_elem(&tls_enhanced_tags, &normalized_tup);
        if (!wrapper_ptr) {
            return NULL;
        }
        tags = &wrapper_ptr->info;
    }
    return tags;
}

// merge_tls_info modifies `this` by merging it with `that`
static __always_inline void merge_tls_info(tls_info_t *this, tls_info_t *that) {
    if (!this || !that) {
        return;
    }

    // Merge chosen_version if not already set
    if (this->chosen_version == 0 && that->chosen_version != 0) {
        this->chosen_version = that->chosen_version;
    }

    // Merge cipher_suite if not already set
    if (this->cipher_suite == 0 && that->cipher_suite != 0) {
        this->cipher_suite = that->cipher_suite;
    }

    // Merge offered_versions bitmask
    this->offered_versions |= that->offered_versions;
}

static __always_inline conn_stats_ts_t *get_conn_stats(conn_tuple_t *t, struct sock *sk) {
    conn_stats_ts_t *cs = bpf_map_lookup_elem(&conn_stats, t);
    if (cs) {
        return cs;
    }

    // initialize-if-no-exist the connection stat, and load it
    conn_stats_ts_t empty = {};
    bpf_memset(&empty, 0, sizeof(conn_stats_ts_t));
    empty.duration_ms = convert_ns_to_ms(bpf_ktime_get_ns());
    empty.cookie = get_sk_cookie(sk);

    // We skip EEXIST because of the use of BPF_NOEXIST flag. Emitting telemetry for EEXIST here spams metrics
    // and do not provide any useful signal since the key is expected to be present sometimes.
    bpf_map_update_with_telemetry(conn_stats, t, &empty, BPF_NOEXIST, -EEXIST);
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

    // Using __get_protocol_stack_if_exists as `conn_tuple_copy` is already normalized.
    protocol_stack_t *protocol_stack = __get_protocol_stack_if_exists(&conn_tuple_copy);
    set_protocol_flag(protocol_stack, FLAG_NPM_ENABLED);
    mark_protocol_direction(t, &conn_tuple_copy, protocol_stack);
    merge_protocol_stacks(&stats->protocol_stack, protocol_stack);

    tls_info_t *tls_tags = get_tls_enhanced_tags(&conn_tuple_copy);
    merge_tls_info(&stats->tls_tags, tls_tags);

    conn_tuple_t *cached_skb_conn_tup_ptr = bpf_map_lookup_elem(&conn_tuple_to_socket_skb_conn_tuple, &conn_tuple_copy);
    if (!cached_skb_conn_tup_ptr) {
        return;
    }

    conn_tuple_copy = *cached_skb_conn_tup_ptr;
    normalize_tuple(&conn_tuple_copy);
    // Using __get_protocol_stack_if_exists as `conn_tuple_copy` is already normalized.
    protocol_stack = __get_protocol_stack_if_exists(&conn_tuple_copy);
    set_protocol_flag(protocol_stack, FLAG_NPM_ENABLED);
    mark_protocol_direction(t, &conn_tuple_copy, protocol_stack);
    merge_protocol_stacks(&stats->protocol_stack, protocol_stack);

    tls_tags = get_tls_enhanced_tags(&conn_tuple_copy);
    merge_tls_info(&stats->tls_tags, tls_tags);
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

    SSL_report_cert(val);

    if (is_protocol_classification_supported()) {
        update_protocol_classification_information(t, val);
    }

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
    val->timestamp_ms = convert_ns_to_ms(ts);

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

    // We skip EEXIST because of the use of BPF_NOEXIST flag. Emitting telemetry for EEXIST here spams metrics
    // and do not provide any useful signal since the key is expected to be present sometimes.
    bpf_map_update_with_telemetry(tcp_stats, t, &empty, BPF_NOEXIST, -EEXIST);

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

    if (stats.failure_reason != 0) {
        val->failure_reason = stats.failure_reason;
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

    // We skip EEXIST because of the use of BPF_NOEXIST flag. Emitting telemetry for EEXIST here spams metrics
    // and do not provide any useful signal since the key is expected to be present sometimes.
    bpf_map_update_with_telemetry(tcp_retransmits, &t, &u32_zero, BPF_NOEXIST, -EEXIST);
    u32 *val = bpf_map_lookup_elem(&tcp_retransmits, &t);
    if (val == NULL) {
        return 0;
    }

    __sync_fetch_and_add(val, count);

    return 0;
}

// handle_congestion_stats snapshots TCP congestion fields from tcp_sock into the
// tcp_congestion_stats map. CO-RE/runtime only; prebuilt is a no-op.
static __always_inline void handle_congestion_stats(conn_tuple_t *t, struct sock *sk) {
#if !defined(COMPILE_PREBUILT)
    tcp_congestion_stats_t empty = {};
    // We skip EEXIST because of the use of BPF_NOEXIST flag. Emitting telemetry for
    // EEXIST here spams metrics and does not provide any useful signal since the key
    // is expected to be present sometimes.
    bpf_map_update_with_telemetry(tcp_congestion_stats, t, &empty, BPF_NOEXIST, -EEXIST);
    tcp_congestion_stats_t *val = bpf_map_lookup_elem(&tcp_congestion_stats, t);
    if (val == NULL) {
        return;
    }
    BPF_CORE_READ_INTO(&val->packets_out, tcp_sk(sk), packets_out);
    BPF_CORE_READ_INTO(&val->lost_out,    tcp_sk(sk), lost_out);
    BPF_CORE_READ_INTO(&val->sacked_out,  tcp_sk(sk), sacked_out);
    BPF_CORE_READ_INTO(&val->delivered,   tcp_sk(sk), delivered);
    BPF_CORE_READ_INTO(&val->retrans_out,  tcp_sk(sk), retrans_out);
    BPF_CORE_READ_INTO(&val->delivered_ce, tcp_sk(sk), delivered_ce);
    BPF_CORE_READ_INTO(&val->bytes_retrans, tcp_sk(sk), bytes_retrans);
    BPF_CORE_READ_INTO(&val->dsack_dups,   tcp_sk(sk), dsack_dups);
    // BPF_CORE_READ_BITFIELD_PROBED requires __builtin_preserve_field_info which is
    // only available with full CO-RE support. The runtime compiler's clang does not
    // provide this builtin, so ca_state is read only on CO-RE (stays 0 on runtime).
    //
    // TODO: add runtime support for ca_state. icsk_ca_state is a 6-bit bitfield
    // (`:6`) inside inet_connection_sock, so you can't take its address directly.
    // The approach: since the runtime tracer compiles against real kernel headers,
    // find the byte offset of the field at compile time, use bpf_probe_read_kernel
    // to read the containing byte, then mask with & 0x3f to extract the low 6 bits.
    // This is fragile across kernel versions if the bitfield layout changes, so it
    // needs validation against a range of kernels before productionizing.
#if defined(COMPILE_CORE)
    struct inet_connection_sock *icsk = &tcp_sk(sk)->inet_conn;
    val->ca_state = (u8)BPF_CORE_READ_BITFIELD_PROBED(icsk, icsk_ca_state);
#endif
#endif
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
    handle_congestion_stats(t, sk);
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
    t.pid = GET_USER_MODE_PID(pid_tgid);
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

__maybe_unused static __always_inline bool tcp_failed_connections_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("tcp_failed_connections_enabled", val);
    return val > 0;
}

// get_tcp_failure returns an error code for tcp_done/tcp_close, if there was one
static __always_inline int get_tcp_failure(struct sock *sk) {
    int err = 0;
    BPF_CORE_READ_INTO(&err, sk, sk_err);
    if (err != 0) {
        return err;
    }

    struct sock_common *skc = (struct sock_common*) sk;
    unsigned char state = 0;
    // the fact that this field is volatile breaks BPF_CORE_READ_INTO, so it must be cast separately
    bpf_probe_read_kernel(&state, 1, (unsigned char*) &skc->skc_state);
    // we are still in SYN_SENT when the socket closed, meaning the connect was cancelled
    if (state == TCP_SYN_SENT) {
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

static __always_inline void report_unrecognized_tcp_failure(int err) {
    // initialize if no-exist
    __u64 one = 1;
    bpf_map_update_with_telemetry(tcp_failure_telemetry, &err, &one, BPF_NOEXIST, -EEXIST);
    __u64 *count = bpf_map_lookup_elem(&tcp_failure_telemetry, &err);
    if (count != NULL) {
        __sync_fetch_and_add(count, one);
    }
}

// handle_tcp_failure handles TCP connection failures on the socket pointer and adds them to the connection tuple
// returns an integer to the caller indicating if there was a failure or not
static __always_inline bool handle_tcp_failure(struct sock *sk, conn_tuple_t *t) {
    if (!tcp_failed_connections_enabled()) {
        return false;
    }
    int err = get_tcp_failure(sk);
    if (err == 0) {
        return false;
    }
    if (is_tcp_failure_recognized(err)) {
        tcp_stats_t stats = { .failure_reason = err };
        update_tcp_stats(t, stats);
        return true;
    }

    report_unrecognized_tcp_failure(err);

    return false;
}

#endif // __TRACER_STATS_H
