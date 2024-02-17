#ifndef __TRACER_EVENTS_H
#define __TRACER_EVENTS_H

#include "bpf_helpers.h"
#include "bpf_telemetry.h"
#include "bpf_builtins.h"

#include "tracer/tracer.h"
#include "tracer/maps.h"
#include "tracer/stats.h"
#include "tracer/telemetry.h"
#include "cookie.h"
#include "ip.h"
#include "port_range.h"
#include "protocols/classification/protocol-classification.h"

#ifdef COMPILE_CORE
#define MSG_PEEK 2
#endif


static __always_inline bool ringbuffers_enabled() {
    __u64 val = 0;
    LOAD_CONSTANT("ringbuffer_enabled", val);
    return val > 0;
}

static __always_inline void clean_protocol_classification(conn_tuple_t *tup) {
    conn_tuple_t conn_tuple = *tup;
    conn_tuple.pid = 0;
    conn_tuple.netns = 0;
    normalize_tuple(&conn_tuple);
    delete_protocol_stack(&conn_tuple, NULL, FLAG_TCP_CLOSE_DELETION);

    conn_tuple_t *skb_tup_ptr = bpf_map_lookup_elem(&conn_tuple_to_socket_skb_conn_tuple, &conn_tuple);
    if (skb_tup_ptr == NULL) {
        return;
    }

    conn_tuple_t skb_tup = *skb_tup_ptr;
    delete_protocol_stack(&skb_tup, NULL, FLAG_TCP_CLOSE_DELETION);
    bpf_map_delete_elem(&conn_tuple_to_socket_skb_conn_tuple, &conn_tuple);
}

static __always_inline conn_flush_t cleanup_conn(void *ctx, conn_tuple_t *tup, struct sock *sk) {
    u32 cpu = bpf_get_smp_processor_id();
    // Will hold the full connection data to send through the perf or ring buffer
    conn_t conn = { .tup = *tup };
    conn_stats_ts_t *cst = NULL;
    tcp_stats_t *tst = NULL;
    u32 *retrans = NULL;
    conn_flush_t conn_flush = { .needs_individual_flush = false };
    bool is_tcp = get_proto(&conn.tup) == CONN_TYPE_TCP;
    bool is_udp = get_proto(&conn.tup) == CONN_TYPE_UDP;

    if (is_tcp) {
        tst = bpf_map_lookup_elem(&tcp_stats, &(conn.tup));
        if (tst) {
            conn.tcp_stats = *tst;
            bpf_map_delete_elem(&tcp_stats, &(conn.tup));
        }

        conn.tup.pid = 0;
        retrans = bpf_map_lookup_elem(&tcp_retransmits, &(conn.tup));
        if (retrans) {
            conn.tcp_retransmits = *retrans;
            bpf_map_delete_elem(&tcp_retransmits, &(conn.tup));
        }
        conn.tup.pid = tup->pid;

        conn.tcp_stats.state_transitions |= (1 << TCP_CLOSE);
    }

    cst = bpf_map_lookup_elem(&conn_stats, &(conn.tup));
    if (is_udp && !cst) {
        increment_telemetry_count(udp_dropped_conns);
        return conn_flush; // nothing to report
    }

    if (cst) {
        conn.conn_stats = *cst;
        bpf_map_delete_elem(&conn_stats, &(conn.tup));
    } else {
        // we don't have any stats for the connection,
        // so cookie is not set, set it here
        conn.conn_stats.cookie = get_sk_cookie(sk);
        // make sure direction is set correctly
        determine_connection_direction(&conn.tup, &conn.conn_stats);
    }

    conn.conn_stats.timestamp = bpf_ktime_get_ns();

    // Batch TCP closed connections before generating a perf event
    batch_t *batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
    if (batch_ptr == NULL) {
        return conn_flush;
    }

    // TODO: Can we turn this into a macro based on TCP_CLOSED_BATCH_SIZE?
    switch (batch_ptr->len) {
    case 0:
        batch_ptr->c0 = conn;
        batch_ptr->len++;
        return conn_flush;
    case 1:
        batch_ptr->c1 = conn;
        batch_ptr->len++;
        return conn_flush;
    case 2:
        batch_ptr->c2 = conn;
        batch_ptr->len++;
        return conn_flush;
    case 3:
        batch_ptr->c3 = conn;
        batch_ptr->len++;
        return conn_flush;
        // In this case the batch is ready to be flushed, which we defer to kretprobe/tcp_close
        // in order to cope with the eBPF stack limitation of 512 bytes.
    }

    // If we hit this section it means we had one or more interleaved tcp_close calls.
    // We send the connection outside of a batch anyway. This is likely not as
    // frequent of a case to cause performance issues and avoid cases where
    // we drop whole connections, which impacts things USM connection matching.
    if (is_tcp) {
        increment_telemetry_count(unbatched_tcp_close);
    }
    if (is_udp) {
        increment_telemetry_count(unbatched_udp_close);
    }
    conn_flush.needs_individual_flush = true;
    conn_flush.conn = conn;
    return conn_flush;
}

static __always_inline conn_flush_t handle_tcp_close(struct pt_regs *ctx) {
    struct sock *sk;
    conn_tuple_t t = {};
    u64 pid_tgid = bpf_get_current_pid_tgid();
    sk = (struct sock *)PT_REGS_PARM1(ctx);

    // Should actually delete something only if the connection never got established & increment counter
    if (bpf_map_delete_elem(&tcp_ongoing_connect_pid, &sk) == 0) {
        increment_telemetry_count(tcp_failed_connect);
    }

    // Get network namespace id
    log_debug("kprobe/tcp_close: tgid: %u, pid: %u", pid_tgid >> 32, pid_tgid & 0xFFFFFFFF);
    if (!read_conn_tuple(&t, sk, pid_tgid, CONN_TYPE_TCP)) {
        return (conn_flush_t){ .needs_individual_flush = false };
    }
    log_debug("kprobe/tcp_close: netns: %u, sport: %u, dport: %u", t.netns, t.sport, t.dport);

    conn_flush_t conn = cleanup_conn(ctx, &t, sk);

    // If protocol classification is disabled, then we don't have kretprobe__tcp_close_clean_protocols hook
    // so, there is no one to use the map and clean it.
    if (is_protocol_classification_supported()) {
        bpf_map_update_with_telemetry(tcp_close_args, &pid_tgid, &t, BPF_ANY);
    }

    return conn;
}


// This function is used to emit a conn_close_event for a single connection that is being closed.
// It is only called on older kernel versions that do not support ring buffers.
__maybe_unused static __always_inline void emit_conn_close_event_perfbuffer(conn_t *conn, void *ctx) {
    if (!conn) {
        return;
    }
    // Here we copy the conn data to a variable allocated in the eBPF stack
    // This is necessary for older Kernel versions only (we validated this behavior on 4.4.0),
    // since you can't directly write a map entry to the perf buffer.
    conn_t conn_copy = {};
    bpf_memcpy(&conn_copy, conn, sizeof(conn_copy));
    u32 cpu = bpf_get_smp_processor_id();
    bpf_perf_event_output(ctx, &conn_close_event, cpu, &conn_copy, sizeof(conn_copy));
}

// This function is used to emit a conn_close_event for a single connection that is being closed.
// It is only called on newer kernel versions that support ring buffers.
__maybe_unused static __always_inline void emit_conn_close_event_ringbuffer(conn_t *conn, void *ctx) {
    u32 cpu = bpf_get_smp_processor_id();
    if (!conn) {
        return;
    }
    if (ringbuffers_enabled()) {
        bpf_ringbuf_output(&conn_close_event, conn, sizeof(*conn), 0);
    } else {
        bpf_perf_event_output(ctx, &conn_close_event, cpu, conn, sizeof(*conn));
    }
}


// This function is used to flush the conn_close_batch to the perf buffer.
// It is only called on older kernel versions that do not support ring buffers.
__maybe_unused static __always_inline void flush_conn_close_if_full_perfbuffer(void *ctx) {
    u32 cpu = bpf_get_smp_processor_id();
    batch_t *batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
    if (!batch_ptr || batch_ptr->len != CONN_CLOSED_BATCH_SIZE) {
        return;
    }

    // Here we copy the batch data to a variable allocated in the eBPF stack
    // This is necessary for older Kernel versions only (we validated this behavior on 4.4.0),
    // since you can't directly write a map entry to the perf buffer.
    batch_t batch_copy = {};
    bpf_memcpy(&batch_copy, batch_ptr, sizeof(batch_copy));
    batch_ptr->len = 0;
    batch_ptr->id++;

    bpf_perf_event_output(ctx, &conn_close_event, cpu, &batch_copy, sizeof(batch_copy));
}


// This function is used to flush the conn_close_batch to the ring buffer.
// It is only called on newer kernel versions that support ring buffers.
__maybe_unused static __always_inline void flush_conn_close_if_full_ringbuffer(void *ctx) {
    u32 cpu = bpf_get_smp_processor_id();
    batch_t *batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
    if (!batch_ptr || batch_ptr->len != CONN_CLOSED_BATCH_SIZE) {
        return;
    }
    if (ringbuffers_enabled()) {
        bpf_ringbuf_output(&conn_close_event, batch_ptr, sizeof(*batch_ptr), 0);
    } else {
        bpf_perf_event_output(ctx, &conn_close_event, cpu, batch_ptr, sizeof(*batch_ptr));
    }
    batch_ptr->len = 0;
    batch_ptr->id++;
}

#endif // __TRACER_EVENTS_H
