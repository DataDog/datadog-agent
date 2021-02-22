#ifndef __TRACER_EVENTS_H
#define __TRACER_EVENTS_H

#include "tracer.h"

#include "tracer-maps.h"
#include "tracer-telemetry.h"
#include "tcp_states.h"

#include "bpf_helpers.h"

static __always_inline int get_proto(conn_tuple_t * t) {
    return (t->metadata & CONN_TYPE_TCP) ? CONN_TYPE_TCP : CONN_TYPE_UDP;
}

static __always_inline void cleanup_conn(conn_tuple_t* tup) {
    u32 cpu = bpf_get_smp_processor_id();

    // Will hold the full connection data to send through the perf buffer
    conn_t conn = { .tup = *tup };
    tcp_stats_t* tst = NULL;
    conn_stats_ts_t* cst = NULL;
    bool is_tcp = get_proto(&conn.tup) == CONN_TYPE_TCP;
    bool is_udp = get_proto(&conn.tup) == CONN_TYPE_UDP;

    // TCP stats don't have the PID
    if (is_tcp) {
        conn.tup.pid = 0;
        tst = bpf_map_lookup_elem(&tcp_stats, &(conn.tup));
        bpf_map_delete_elem(&tcp_stats, &(conn.tup));
        conn.tup.pid = tup->pid;

        if (tst) {
            conn.tcp_stats = *tst;
        }

        conn.tcp_stats.state_transitions |= (1 << TCP_CLOSE);
    }

    cst = bpf_map_lookup_elem(&conn_stats, &(conn.tup));
    // Delete this connection from our stats map
    bpf_map_delete_elem(&conn_stats, &(conn.tup));

    if (cst) {
        cst->timestamp = bpf_ktime_get_ns();
        conn.conn_stats = *cst;
    }

    // Batch TCP closed connections before generating a perf event
    batch_t* batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
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
        return;
    case 4:
        // In this case the batch is ready to be flushed, which we defer to kretprobe/tcp_close
        // in order to cope with the eBPF stack limitation of 512 bytes.
        batch_ptr->c4 = conn;
        batch_ptr->len++;
        return;
    }

    // If we hit this section it means we had one or more interleaved tcp_close calls.
    // This could result in a missed tcp_close event, so we track it using our telemetry map.
    if (is_tcp) increment_telemetry_count(missed_tcp_close);
    if (is_udp) increment_telemetry_count(missed_udp_close);
}

static __always_inline void flush_conn_close_if_full(struct pt_regs * ctx) {
    u32 cpu = bpf_get_smp_processor_id();
    batch_t * batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
    if (!batch_ptr) {
        return;
    }

    if (batch_ptr->len == CONN_CLOSED_BATCH_SIZE) {
        // Here we copy the batch data to a variable allocated in the eBPF stack
        // This is necessary for older Kernel versions only (we validated this behavior on 4.4.0),
        // since you can't directly write a map entry to the perf buffer.
        batch_t batch_copy = {};
        __builtin_memcpy(&batch_copy, batch_ptr, sizeof(batch_copy));
        batch_ptr->len = 0;
        batch_ptr->id++;
        bpf_perf_event_output(ctx, &conn_close_event, cpu, &batch_copy, sizeof(batch_copy));
    }
}

#endif // __TRACER_EVENTS_H
