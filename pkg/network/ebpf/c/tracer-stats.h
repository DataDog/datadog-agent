#ifndef __TRACER_STATS_H
#define __TRACER_STATS_H

#include "tracer.h"
#include "tracer-maps.h"
#include "tracer-telemetry.h"

static int read_conn_tuple(conn_tuple_t *t, struct sock *skp, u64 pid_tgid, metadata_mask_t type);

static __always_inline u32 get_sk_cookie(struct sock *sk) {
    u64 t = bpf_ktime_get_ns();
    return (u32)((u64)sk ^ t);
}

static __always_inline conn_stats_ts_t *get_conn_stats(conn_tuple_t *t, struct sock *sk) {
    // initialize-if-no-exist the connection stat, and load it
    conn_stats_ts_t empty = {};
    __builtin_memset(&empty, 0, sizeof(conn_stats_ts_t));
    empty.cookie = get_sk_cookie(sk);
    //if (bpf_map_update_elem(conn_stats, t, &empty, BPF_NOEXIST) == -E2BIG) {
    //    increment_telemetry_count(conn_stats_max_entries_hit);
    //}
    bpf_map_update_elem(conn_stats, t, &empty, BPF_NOEXIST);
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

static __always_inline void update_conn_stats(conn_tuple_t *t, size_t sent_bytes, size_t recv_bytes, u64 ts, conn_direction_t dir,
    __u32 packets_out, __u32 packets_in, packet_count_increment_t segs_type, struct sock *sk) {
    conn_stats_ts_t *val;

    val = get_conn_stats(t, sk);
    if (!val) {
        return;
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
    val->timestamp = ts;

    if (dir != CONN_DIRECTION_UNKNOWN) {
        val->direction = dir;
    } else if (val->direction == CONN_DIRECTION_UNKNOWN) {
        u32 *port_count = NULL;
        port_binding_t pb = {};
        pb.port = t->sport;
        if (t->metadata & CONN_TYPE_TCP) {
            pb.netns = t->netns;
            port_count = bpf_map_lookup_elem(&port_bindings, &pb);
        } else {
            port_count = bpf_map_lookup_elem(&udp_port_bindings, &pb);
        }
        val->direction = (port_count != NULL && *port_count > 0) ? CONN_DIRECTION_INCOMING : CONN_DIRECTION_OUTGOING;
    }
}

static __always_inline void update_tcp_stats(conn_tuple_t *t, tcp_stats_t stats) {
    // query stats without the PID from the tuple
    __u32 pid = t->pid;
    t->pid = 0;

    // initialize-if-no-exist the connetion state, and load it
    tcp_stats_t empty = {};
    bpf_map_update_elem(tcp_stats, t, &empty, BPF_NOEXIST);

    tcp_stats_t *val = bpf_map_lookup_elem(&tcp_stats, t);
    t->pid = pid;
    if (val == NULL) {
        return;
    }

    if (stats.retransmits > 0) {
        __sync_fetch_and_add(&val->retransmits, stats.retransmits);
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

static __always_inline int handle_retransmit(struct sock *sk, int segs) {
    conn_tuple_t t = {};
    u64 zero = 0;

    if (!read_conn_tuple(&t, sk, zero, CONN_TYPE_TCP)) {
        return 0;
    }

    tcp_stats_t stats = { .retransmits = segs, .rtt = 0, .rtt_var = 0 };
    update_tcp_stats(&t, stats);

    return 0;
}

#endif // __TRACER_STATS_H
