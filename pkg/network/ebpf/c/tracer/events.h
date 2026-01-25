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

#ifdef COMPILE_CORE
#define MSG_PEEK 2
#endif

static __always_inline void clean_protocol_classification(conn_tuple_t *tup) {
    conn_tuple_t conn_tuple = *tup;
#ifdef COMPILE_PREBUILT
    conn_tuple.pid = 0;
#endif // COMPILE_PREBUILT
    conn_tuple.netns = 0;
    normalize_tuple(&conn_tuple);
    delete_protocol_stack(&conn_tuple, NULL, FLAG_TCP_CLOSE_DELETION);
    bpf_map_delete_elem(&tls_enhanced_tags, &conn_tuple);

    conn_tuple_t *skb_tup_ptr = bpf_map_lookup_elem(&conn_tuple_to_socket_skb_conn_tuple, &conn_tuple);
    if (skb_tup_ptr == NULL) {
        return;
    }

    conn_tuple_t skb_tup = *skb_tup_ptr;
    delete_protocol_stack(&skb_tup, NULL, FLAG_TCP_CLOSE_DELETION);
    bpf_map_delete_elem(&tls_enhanced_tags, &skb_tup);
    bpf_map_delete_elem(&conn_tuple_to_socket_skb_conn_tuple, &conn_tuple);
}

static __always_inline bool is_batching_enabled() {
    __u64 batching_enabled = 0;
    LOAD_CONSTANT("batching_enabled", batching_enabled);
    return batching_enabled != 0;
}

__maybe_unused static __always_inline __u64 get_ringbuf_flags(size_t data_size) {
    if (is_batching_enabled()) {
        return 0;
    }

    __u64 ringbuffer_wakeup_size = 0;
    LOAD_CONSTANT("ringbuffer_wakeup_size", ringbuffer_wakeup_size);
    if (ringbuffer_wakeup_size == 0) {
        return 0;
    }

    __u64 sz = bpf_ringbuf_query(&conn_close_event, DD_BPF_RB_AVAIL_DATA);
    return (sz + data_size) >= ringbuffer_wakeup_size ? DD_BPF_RB_FORCE_WAKEUP : DD_BPF_RB_NO_WAKEUP;
}

__maybe_unused static __always_inline void submit_closed_conn_event(void *ctx, int cpu, void *event_data, size_t data_size) {
    __u64 ringbuffers_enabled = 0;
    LOAD_CONSTANT("ringbuffers_enabled", ringbuffers_enabled);
    if (ringbuffers_enabled > 0) {
        bpf_ringbuf_output(&conn_close_event, event_data, data_size, get_ringbuf_flags(data_size));
    } else {
        bpf_perf_event_output(ctx, &conn_close_event, cpu, event_data, data_size);
    }
}

static __always_inline int cleanup_conn(void *ctx, conn_tuple_t *tup, struct sock *sk) {
    u32 cpu = bpf_get_smp_processor_id();
    // Will hold the full connection data to send through the perf or ring buffer
    conn_t conn = { .tup = *tup };
    conn_stats_ts_t *cst = NULL;
    tcp_stats_t *tst = NULL;
    u32 *retrans = NULL;
    bool is_tcp = get_proto(&conn.tup) == CONN_TYPE_TCP;
    bool is_udp = get_proto(&conn.tup) == CONN_TYPE_UDP;
    bool cst_flushable = false;

    cst = bpf_map_lookup_elem(&conn_stats, &(conn.tup));
    // if we were able to delete the entry, it signals that no other threads have flushed it
    if (cst && (bpf_map_delete_elem(&conn_stats, &(conn.tup)) == 0)) {
        cst_flushable = true;
        conn.conn_stats = *cst;
    }

    if (is_udp && !cst_flushable) {
        increment_telemetry_count(udp_dropped_conns);
        return -1;
    }

    if (is_tcp) {
        tst = bpf_map_lookup_elem(&tcp_stats, &(conn.tup));
        if (tst && (bpf_map_delete_elem(&tcp_stats, &(conn.tup)) == 0)) {
            conn.tcp_stats = *tst;
        } else {
            if (!cst_flushable) {
                int *count = bpf_map_lookup_elem(&tcp_retransmits, &(conn.tup));
                if (count) {
                    increment_telemetry_count_times(tcp_syn_retransmit, *count);
                    bpf_map_delete_elem(&tcp_retransmits, &(conn.tup));
                }
                return -1;
            }
        }

        conn.tup.pid = 0;
        retrans = bpf_map_lookup_elem(&tcp_retransmits, &(conn.tup));
        if (retrans) {
            conn.tcp_stats.retransmits = *retrans;
            bpf_map_delete_elem(&tcp_retransmits, &(conn.tup));
        }
        conn.tup.pid = tup->pid;
        conn.tcp_stats.state_transitions |= (1 << TCP_CLOSE);

        if (sk) {
            __u32 packets_in = 0;
            __u32 packets_out = 0;
            __u32 total_retrans = 0;
            get_tcp_segment_counts(sk, &packets_in, &packets_out);
            get_tcp_retrans_counts(sk, &total_retrans);

            if (packets_out > conn.conn_stats.sent_packets) {
                conn.conn_stats.sent_packets = packets_out;
            }
            if (packets_in > conn.conn_stats.recv_packets) {
                conn.conn_stats.recv_packets = packets_in;
            }
            if (total_retrans > conn.tcp_stats.retransmits) {
                conn.tcp_stats.retransmits = total_retrans;
            }
        }
    }

    // update the `duration` field to reflect the duration of the
    // connection; `duration` had the creation timestamp for
    // the conn_stats_ts_t object up to now. we re-use this field
    // for the duration since we would overrun stack size limits
    // if we added another field
    __u64 start_ns = convert_ms_to_ns(conn.conn_stats.duration_ms);
    __u64 delta_ns = bpf_ktime_get_ns() - start_ns;
    conn.conn_stats.duration_ms = convert_ns_to_ms(delta_ns);

    if (is_batching_enabled()) {
        // Batch TCP closed connections before generating a perf event
        batch_t *batch_ptr = bpf_map_lookup_elem(&conn_close_batch, &cpu);
        if (batch_ptr == NULL) {
            return -1;
        }

        // TODO: Can we turn this into a macro based on TCP_CLOSED_BATCH_SIZE?
        switch (batch_ptr->len) {
        case 0:
            batch_ptr->c0 = conn;
            batch_ptr->len++;
            return 0;
        case 1:
            batch_ptr->c1 = conn;
            batch_ptr->len++;
            return 0;
        case 2:
            batch_ptr->c2 = conn;
            batch_ptr->len++;
            return 0;
        case 3:
            batch_ptr->c3 = conn;
            batch_ptr->len++;
            // In this case the batch is ready to be flushed, which we defer to kretprobe/tcp_close
            // in order to cope with the eBPF stack limitation of 512 bytes.
            return 0;
        }
    }

    // If we hit this section it means we had one or more interleaved tcp_close calls.
    // We send the connection outside of a batch anyway. This is likely not as
    // frequent of a case to cause performance issues and avoid cases where
    // we drop whole connections, which impacts things USM connection matching.
    submit_closed_conn_event(ctx, cpu, &conn, sizeof(conn_t));
    if (is_batching_enabled()) {
        if (is_tcp) {
            increment_telemetry_count(unbatched_tcp_close);
        }
        if (is_udp) {
            increment_telemetry_count(unbatched_udp_close);
        }
    }
    return 0;
}

static __always_inline void flush_conn_close_if_full(void *ctx) {
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

    submit_closed_conn_event(ctx, cpu, &batch_copy, sizeof(batch_t));
}

#endif // __TRACER_EVENTS_H
