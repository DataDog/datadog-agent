#ifndef __TRACER_EVENTS_H
#define __TRACER_EVENTS_H

#include "bpf_helpers.h"
#include "bpf_telemetry.h"
#include "bpf_builtins.h"

#include "tracer.h"
#include "tracer-maps.h"
#include "tracer-stats.h"
#include "tracer-telemetry.h"
#include "cookie.h"
#include "protocols/classification/tracer-maps.h"
#include "ip.h"

#ifdef COMPILE_CORE
#define MSG_PEEK 2
#endif

static __always_inline void clean_protocol_classification(conn_tuple_t *tup) {
    conn_tuple_t conn_tuple = *tup;
    conn_tuple.pid = 0;
    conn_tuple.netns = 0;
    bpf_map_delete_elem(&connection_protocol, &conn_tuple);
    bpf_map_delete_elem(&tls_connection, &conn_tuple);

    conn_tuple_t *skb_tup_ptr = bpf_map_lookup_elem(&conn_tuple_to_socket_skb_conn_tuple, &conn_tuple);
    if (skb_tup_ptr != NULL) {
        conn_tuple_t skb_tup = *skb_tup_ptr;
        conn_tuple_t inverse_skb_conn_tup = *skb_tup_ptr;
        flip_tuple(&inverse_skb_conn_tup);
        inverse_skb_conn_tup.pid = 0;
        inverse_skb_conn_tup.netns = 0;
        bpf_map_delete_elem(&connection_protocol, &inverse_skb_conn_tup);
        bpf_map_delete_elem(&connection_protocol, &skb_tup);
        bpf_map_delete_elem(&tls_connection, &inverse_skb_conn_tup);
        bpf_map_delete_elem(&tls_connection, &skb_tup);
    }

    bpf_map_delete_elem(&conn_tuple_to_socket_skb_conn_tuple, &conn_tuple);
}

static __always_inline void cleanup_conn(conn_tuple_t *tup, struct sock *sk) {
    clean_protocol_classification(tup);

    u32 cpu = bpf_get_smp_processor_id();

    // Will hold the full connection data to send through the perf buffer
    conn_t conn = { .tup = *tup };
    conn_stats_ts_t *cst = NULL;
    bool is_tcp = get_proto(&conn.tup) == CONN_TYPE_TCP;
    bool is_udp = get_proto(&conn.tup) == CONN_TYPE_UDP;

    // TCP stats don't have the PID
    if (is_tcp) {
        conn.tup.pid = 0;
        tcp_stats_t *tst = bpf_map_lookup_elem(&tcp_stats, &(conn.tup));
        if (tst) {
            conn.tcp_stats = *tst;
            bpf_map_delete_elem(&tcp_stats, &(conn.tup));
        }
        conn.tup.pid = tup->pid;

        conn.tcp_stats.state_transitions |= (1 << TCP_CLOSE);
    }

    cst = bpf_map_lookup_elem(&conn_stats, &(conn.tup));
    if (!cst && is_udp) {
        increment_telemetry_count(udp_dropped_conns);
        return; // nothing to report
    }

    if (cst) {
        conn.conn_stats = *cst;
        bpf_map_delete_elem(&conn_stats, &(conn.tup));
    } else {
        // we don't have any stats for the connection,
        // so cookie is not set, set it here
        conn.conn_stats.cookie = get_sk_cookie(sk);
    }

    conn.conn_stats.timestamp = bpf_ktime_get_ns();

    // Batch TCP closed connections before generating a perf event
    batch_t *batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
    if (batch_ptr == NULL) {
        return;
    }

    // TODO: Can we turn this into a macro based on TCP_CLOSED_BATCH_SIZE?
    switch (batch_ptr->len) {
    case 0:
        batch_ptr->c0 = conn;
        batch_ptr->len++;
        return;
    case 1:
        batch_ptr->c1 = conn;
        batch_ptr->len++;
        return;
    case 2:
        batch_ptr->c2 = conn;
        batch_ptr->len++;
        return;
    case 3:
        batch_ptr->c3 = conn;
        batch_ptr->len++;
        // In this case the batch is ready to be flushed, which we defer to kretprobe/tcp_close
        // in order to cope with the eBPF stack limitation of 512 bytes.
        return;
    }

    // If we hit this section it means we had one or more interleaved tcp_close calls.
    // This could result in a missed tcp_close event, so we track it using our telemetry map.
    if (is_tcp) {
        increment_telemetry_count(missed_tcp_close);
    }
    if (is_udp) {
        increment_telemetry_count(missed_udp_close);
    }
}

static __always_inline void flush_conn_close_if_full(void *ctx) {
    u32 cpu = bpf_get_smp_processor_id();
    batch_t *batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
    if (!batch_ptr) {
        return;
    }

    if (batch_ptr->len == CONN_CLOSED_BATCH_SIZE) {
        // Here we copy the batch data to a variable allocated in the eBPF stack
        // This is necessary for older Kernel versions only (we validated this behavior on 4.4.0),
        // since you can't directly write a map entry to the perf buffer.
        batch_t batch_copy = {};
        bpf_memcpy(&batch_copy, batch_ptr, sizeof(batch_copy));
        batch_ptr->len = 0;
        batch_ptr->id++;

        // we cannot use the telemetry macro here because of stack size constraints
        bpf_perf_event_output(ctx, &conn_close_event, cpu, &batch_copy, sizeof(batch_copy));
    }
}

#endif // __TRACER_EVENTS_H
